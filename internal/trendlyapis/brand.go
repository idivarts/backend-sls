package trendlyapis

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"

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

type IBrandMember struct {
	BrandID string  `json:"brandId" binding:"required"`
	Email   string  `json:"email" binding:"required"`
	Name    *string `json:"name"`
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

	bManager := &trendlymodels.BrandMember{
		ManagerID: userRecord.UID,
		Role:      "user",
		Status:    0,
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
