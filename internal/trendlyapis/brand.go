package trendlyapis

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"firebase.google.com/go/v4/auth"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	myjwt "github.com/idivarts/backend-sls/internal/trendlyapis/jwt"
	"github.com/idivarts/backend-sls/pkg/firebase/fauth"
)

type IBrandMember struct {
	BrandID string `json:"brandId" binding:"required"`
	Email   string `json:"email" binding:"required"`
}

func CreateBrandMember(c *gin.Context) {
	var req IBrandMember
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userRecord, err := fauth.Client.GetUserByEmail(context.Background(), req.Email)

	if err != nil {
		userToCreate := (&auth.UserToCreate{}).Email(req.Email).EmailVerified(false)

		userRecord, err = fauth.Client.CreateUser(context.Background(), userToCreate)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// fauth.Client.EmailSignInLink()
	link, err := GenerateInvitationLink(userRecord.Email, userRecord.EmailVerified, req.BrandID, userRecord.UID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully parsed JSON", "user": userRecord, "link": link})
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

type FirebaseActionRequest struct {
	Token   string `form:"token" binding:"required"`   // Out-of-band code for the action
	BrandID string `form:"brandId" binding:"required"` // Operation mode (e.g., resetPassword, verifyEmail)
}

func ValidateFirebaseCallback(c *gin.Context) {
	var req FirebaseActionRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	uid, err := myjwt.DecodeUID(req.Token)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	uRecord, err := fauth.Client.GetUser(context.Background(), uid)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	bmember := trendlymodels.BrandMember{
		ManagerID: uid,
		Role:      "user",
		Status:    1,
	}
	_, err = bmember.Set(req.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s?email=%s", os.Getenv("SELF_BASE_URL"), uRecord.Email))
}
