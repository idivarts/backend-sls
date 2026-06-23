package trendlyapis

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"cloud.google.com/go/firestore"
	"firebase.google.com/go/v4/auth"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/constants"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	myjwt "github.com/idivarts/backend-sls/internal/trendlyapis/jwt"
	"github.com/idivarts/backend-sls/pkg/firebase/fauth"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"github.com/idivarts/backend-sls/pkg/myemail"
	"github.com/idivarts/backend-sls/pkg/myutil"
	"github.com/idivarts/backend-sls/templates"
)

// IBrand is the request body for POST /api/v2/brands/create.
//
// It serves three flows through a single endpoint:
//   - Create + finalize a brand in one shot: send Brand fields, omit BrandID,
//     leave Draft=false. Used by the onboarding form and landing create-brand.
//   - Create a draft (Firestore-only, no provisioning): send Brand fields,
//     omit BrandID, set Draft=true. Used by the AI onboarding chat which needs
//     a brandId to scope its conversation before the user has finished.
//   - Finalize an existing draft: send BrandID, leave Draft=false. Used by the
//     AI onboarding chat at the end of the conversation.
//
// The response is always { brandId, message }, so callers can read the new id
// for create flows and fetch the brand document from Firestore client-side.
type IBrand struct {
	BrandID *string              `json:"brandId,omitempty"`
	Brand   *trendlymodels.Brand `json:"brand,omitempty"`
	Draft   bool                 `json:"draft,omitempty"`
}

func CreateBrand(c *gin.Context) {
	var req IBrand
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	creator, ok := middlewares.GetUserId(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not found"})
		return
	}

	brandID := ""
	if req.BrandID != nil {
		brandID = *req.BrandID
	}

	// Path A: no brandId — create a fresh brand document (+ member doc) from
	// the supplied Brand fields. Frontend never touches Firestore for brand
	// creation anymore; the id is allocated server-side and returned.
	if brandID == "" {
		if req.Brand == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Either brandId or brand fields are required"})
			return
		}

		newBrand := *req.Brand
		// Server controls lifecycle/derived fields regardless of what the
		// client sent. A freshly created brand is always a draft; finalize
		// (below) is what flips OnboardingComplete to true.
		newBrand.OnboardingComplete = false
		newBrand.HasPayWall = false
		newBrand.UnlockedInfluencers = nil
		newBrand.DiscoveredInfluencers = nil
		newBrand.PostedCollaborations = nil
		newBrand.Backend = nil

		// Stamp creationTime server-side if the client didn't send one, so every
		// brand has a creation timestamp regardless of which create flow was used.
		if newBrand.CreationTime == nil {
			now := time.Now().UnixMilli()
			newBrand.CreationTime = &now
		}

		brandDocRef := firestoredb.Client.Collection("brands").NewDoc()
		brandID = brandDocRef.ID
		memberDocRef := brandDocRef.Collection("members").Doc(creator)
		member := trendlymodels.BrandMember{
			ManagerID: creator,
			Status:    0,
		}

		// Marshal Brand the same way Brand.Insert does, so json tags (and
		// `omitempty`) are honoured inside the transaction's tx.Set.
		brandBytes, err := json.Marshal(&newBrand)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Error encoding brand"})
			return
		}
		var brandData map[string]interface{}
		if err := json.Unmarshal(brandBytes, &brandData); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Error encoding brand"})
			return
		}

		if newBrand.OrganizationID != nil && *newBrand.OrganizationID != "" {
			// Brand is being created INSIDE an existing organization. Enforce
			// the org's brand limit ATOMICALLY with the brand+member writes —
			// otherwise a failed limit check would leave an orphan brand doc
			// behind. Same transaction also pre-reserves the slot in
			// org.brandIds so the later finalize step is a no-op for it.
			orgId := *newBrand.OrganizationID
			ctx := context.Background()
			orgRef := firestoredb.Client.Collection("organizations").Doc(orgId)
			txErr := firestoredb.Client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
				orgSnap, err := tx.Get(orgRef)
				if err != nil {
					return err
				}
				var org trendlymodels.Organization
				if err := orgSnap.DataTo(&org); err != nil {
					return err
				}
				if org.DeletedAt != nil {
					return errBrandLimit
				}
				limit := org.MaxBrands
				if limit <= 0 {
					limit = trendlymodels.ResolveMaxBrands(myutil.DerefString(org.PlanKey))
				}
				if len(org.BrandIds) >= limit {
					return errBrandLimit
				}
				if err := tx.Set(brandDocRef, brandData, firestore.MergeAll); err != nil {
					return err
				}
				if err := tx.Set(memberDocRef, member); err != nil {
					return err
				}
				return tx.Update(orgRef, []firestore.Update{
					{Path: "brandIds", Value: firestore.ArrayUnion(brandID)},
				})
			})
			if txErr == errBrandLimit {
				c.JSON(http.StatusConflict, gin.H{"error": "brand-limit-reached", "message": "Your organization has reached its plan's brand limit. Upgrade to add more brands."})
				return
			}
			if txErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": txErr.Error(), "message": "Error creating brand"})
				return
			}
		} else {
			// No explicit org attachment — finalize will provision a personal
			// org for this brand later. Brand + member writes happen serially
			// (no org limit to coordinate against here).
			if _, err := newBrand.Insert(brandID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Error creating brand"})
				return
			}
			if _, err := member.Set(brandID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Error creating brand member"})
				return
			}
		}
	}

	// Path B: caller asked for a draft only — skip provisioning. The AI chat
	// will call back with finalize (Draft=false) when the conversation ends.
	if req.Draft {
		c.JSON(http.StatusOK, gin.H{"brandId": brandID, "message": "Draft brand created"})
		return
	}

	// Path C: finalize. Provision the org, default team, and flip
	// OnboardingComplete. Same logic the endpoint had before the create-mode
	// was folded in — just gated on Draft now.
	brand := trendlymodels.Brand{}
	if err := brand.Get(brandID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid Input"})
		return
	}

	// Idempotent finalize: a brand whose onboarding already completed must not be
	// re-provisioned (which would reset billing/credits). The AI onboarding flow
	// creates a draft first and calls this once at the end, so guard against any
	// duplicate finalize call.
	if brand.OnboardingComplete {
		c.JSON(http.StatusOK, gin.H{"brandId": brandID, "message": "Brand already initiated"})
		return
	}

	brand.OnboardingComplete = true

	// Billing lives on the Organization — nothing to stamp on the brand here.

	// Old per-brand credits removed — no credit stamping on provisioning. The
	// new org-level token wallet is seeded by the Credit System ticket.
	brand.HasPayWall = false

	// Auto-provision a personal organization for any brand finalized without
	// one, so billing/plan/credits (which live on the Organization) always have
	// a home and the paywall gate has a billing entity to read. Guarded on
	// OrganizationID so re-finalize is idempotent and brands created inside an
	// org (AddBrandToOrganization) keep their existing org.
	if brand.OrganizationID == nil || *brand.OrganizationID == "" {
		orgName := "My Organization"
		orgId, _, perr := provisionPersonalOrg(creator, orgName, brand.Image, []string{brandID})
		if perr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": perr.Error(), "message": "Error provisioning organization"})
			return
		}
		brand.OrganizationID = &orgId
	}
	// If OrganizationID is already set, the writer that set it (Path A above or
	// AddBrandToOrganization) atomically added brandID to org.BrandIds in the
	// same transaction, so no re-attach is needed here.

	if _, err := brand.Insert(brandID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error in inserting"})
		return
	}

	// Every brand gets a default team that owns all connected socials initially.
	if _, err := trendlymodels.EnsureDefaultTeam(brandID, creator, time.Now().UnixMilli()); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error creating default team"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"brandId": brandID, "message": "Successfully iniated the brand"})
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
	// Use the stage-aware backend base (adds the "/dev" API Gateway stage in dev)
	// so invite-acceptance links resolve to the correct environment.
	link := fmt.Sprintf("%s/firebase/brands/members/add?brandId=%s&token=%s", constants.GetTrendlyBE(), url.QueryEscape(brandId), url.QueryEscape(token))

	return link
}
