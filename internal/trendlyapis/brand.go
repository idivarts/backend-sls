package trendlyapis

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"firebase.google.com/go/v4/auth"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	myjwt "github.com/idivarts/backend-sls/internal/trendlyapis/jwt"
	"github.com/idivarts/backend-sls/pkg/firebase/fauth"
	"github.com/idivarts/backend-sls/pkg/myemail"
	"github.com/idivarts/backend-sls/pkg/myutil"
	"github.com/idivarts/backend-sls/templates"
)

type IBrand struct {
	BrandID string `json:"brandId" binding:"required"`
}

func CreateBrand(c *gin.Context) {
	var req IBrand
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	brand := trendlymodels.Brand{}
	err := brand.Get(req.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid Input"})
		return
	}

	// Idempotent finalize: a brand whose onboarding already completed must not be
	// re-provisioned (which would reset billing/credits). The AI onboarding flow
	// creates a draft first and calls this once at the end, so guard against any
	// duplicate finalize call.
	if brand.OnboardingComplete {
		c.JSON(http.StatusOK, gin.H{"message": "Brand already initiated"})
		return
	}

	brand.OnboardingComplete = true

	// Billing lives on the Organization — nothing to stamp on the brand here.

	// Old per-brand credits removed — no credit stamping on provisioning. The
	// new org-level token wallet is seeded by the Credit System ticket.
	brand.HasPayWall = false

	creator, _ := middlewares.GetUserId(c)

	// Auto-provision a personal organization for any brand finalized without
	// one, so billing/plan/credits (which live on the Organization) always have
	// a home and the paywall gate has a billing entity to read. Guarded on
	// OrganizationID so re-finalize is idempotent and brands created inside an
	// org (AddBrandToOrganization) keep their existing org.
	if brand.OrganizationID == nil || *brand.OrganizationID == "" {
		orgName := "My Organization"
		orgId, _, perr := provisionPersonalOrg(creator, orgName, brand.Image, []string{req.BrandID})
		if perr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": perr.Error(), "message": "Error provisioning organization"})
			return
		}
		brand.OrganizationID = &orgId
	}

	_, err = brand.Insert(req.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error in inserting"})
		return
	}

	// Every brand gets a default team that owns all connected socials initially.
	if _, err := trendlymodels.EnsureDefaultTeam(req.BrandID, creator, time.Now().UnixMilli()); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error creating default team"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully iniated the brand"})
}

type IBrandMember struct {
	BrandID string  `json:"brandId" binding:"required"`
	Email   string  `json:"email" binding:"required"`
	Name    *string `json:"name"`
	// TeamID is the single team to add the invited member to. Empty assigns the
	// brand's default team. The member inherits that team's feature privileges.
	TeamID *string `json:"teamId"`
}

func CreateBrandMember(c *gin.Context) {
	var req IBrandMember
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userId, b := middlewares.GetUserId(c)
	if !b {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User not found"})
		return
	}
	user := middlewares.GetUserObject(c)
	inviterName := user["name"].(string)

	cUser := &trendlymodels.BrandMember{}
	err := cUser.Get(req.BrandID, userId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "User not a part of brand", "error": err.Error()})
		return
	}

	// Inviter must be allowed to manage members (brand_admin:members). Legacy
	// members (pre-migration, no team) are permitted through during the
	// transition — remove this fallback once scripts/migrate-teams-v2 has run.
	cTeam, terr := cUser.ResolveTeam(req.BrandID)
	if terr != nil {
		c.JSON(http.StatusForbidden, gin.H{"message": "Unable to resolve your team", "error": terr.Error()})
		return
	}
	if cTeam != nil && !cTeam.HasPrivilege(trendlymodels.FeatureBrandAdmin, trendlymodels.PrivAdminMembers) {
		c.JSON(http.StatusForbidden, gin.H{"message": "You don't have permission to manage members"})
		return
	}

	// Resolve the team to add the member to (default: the brand's default team).
	var teamID string
	if req.TeamID != nil && *req.TeamID != "" {
		target := &trendlymodels.Team{}
		if err := target.Get(req.BrandID, *req.TeamID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Target team not found"})
			return
		}
		teamID = *req.TeamID
	} else {
		defTeams, derr := trendlymodels.EnsureDefaultTeam(req.BrandID, userId, time.Now().UnixMilli())
		if derr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": derr.Error(), "message": "Unable to resolve default team"})
			return
		}
		teamID = defTeams[0].ID
	}

	brand := &trendlymodels.Brand{}
	err = brand.Get(req.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Can't find the brand"})
		return
	}

	userRecord, err := fauth.Client.GetUserByEmail(context.Background(), req.Email)

	if err != nil {
		userToCreate := (&auth.UserToCreate{}).Email(req.Email).EmailVerified(false)
		if req.Name != nil {
			userToCreate = userToCreate.DisplayName(*req.Name)
		}

		userRecord, err = fauth.Client.CreateUser(context.Background(), userToCreate)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error creating User Record"})
			return
		}
	}

	// Adding someone to a brand also makes them a member of that brand's
	// organization (as OrgRoleMember). Do this BEFORE creating the brand-member
	// doc so a seat-capped org rejects the invite instead of half-adding the
	// member. No-op if the brand has no org or they're already an org member.
	orgId := ""
	if brand.OrganizationID != nil {
		orgId = *brand.OrganizationID
	}
	if err := EnsureOrgMembership(orgId, userRecord.UID); err != nil {
		if err == errSeatLimit {
			c.JSON(http.StatusConflict, gin.H{"error": "org-seat-limit-reached", "message": "Your organization has reached its plan's member limit. Upgrade to add more members."})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Unable to add member to organization"})
		return
	}

	bManager := &trendlymodels.BrandMember{
		ManagerID: userRecord.UID,
		Status:    0,
		TeamID:    teamID,
	}
	_, err = bManager.Set(req.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Unable to insert Brand Member"})
		return
	}

	manager := trendlymodels.Manager{}
	err = manager.Get(userRecord.UID)
	if err != nil {
		manager = trendlymodels.Manager{
			Name:            myutil.DerefString(req.Name),
			Email:           req.Email,
			IsAdmin:         false,
			IsChatConnected: false,
			Settings: &trendlymodels.ManagerSettings{
				Theme:             "light",
				EmailNotification: true,
				PushNotification:  true,
			},
			PushNotificationToken: trendlymodels.PushNotificationToken{
				IOS:     []string{},
				Android: []string{},
				Web:     []string{},
			},
		}
		_, err = manager.Insert(userRecord.UID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Unable to insert Manager"})
			return
		}
	}

	// fauth.Client.EmailSignInLink()
	link, err := GenerateInvitationLink(userRecord.Email, userRecord.EmailVerified, req.BrandID, userRecord.UID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 	<!--
	//   Dynamic Variables:
	//     {{.RecipientName}}   => Name of the invited team member
	//     {{.InviterName}}     => Name of the person who invited them
	//     {{.BrandName}}       => Name of the brand
	//     {{.AcceptLink}}      => Link to accept the invitation and join the brand
	// -->
	data := map[string]interface{}{
		"RecipientName": req.Name,
		"InviterName":   inviterName,
		"BrandName":     brand.Name,
		"AcceptLink":    link,
	}
	err = myemail.SendCustomHTMLEmail(userRecord.Email, templates.BrandEmailInvite, templates.SubjectBrandEmailInvite, data)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error sending email"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully parsed JSON", "user": userRecord, "manager": manager, "link": link})
}

// GenerateInvitationLink creates a password reset link
func GenerateInvitationLink(email string, userVerified bool, brandId string, uid string) (string, error) {
	actionCodeSettings := &auth.ActionCodeSettings{
		URL:             getRedirectLink(brandId, uid),
		HandleCodeInApp: true,
	}
	if userVerified {
		link, err := fauth.Client.EmailVerificationLinkWithSettings(context.Background(), email, actionCodeSettings)
		return link, err
	} else {
		link, err := fauth.Client.PasswordResetLinkWithSettings(context.Background(), email, actionCodeSettings)
		return link, err
	}
}

// This will be used to get the link to redirect
func getRedirectLink(brandId, uid string) string {
	token, err := myjwt.EncodeUID(uid)
	if err != nil {
		panic("Error Creating custom token")
	}
	link := fmt.Sprintf("%s/firebase/brands/members/add?brandId=%s&token=%s", os.Getenv("SELF_BASE_URL"), url.QueryEscape(brandId), url.QueryEscape(token))

	return link
}
