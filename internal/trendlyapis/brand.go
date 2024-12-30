package trendlyapis

import (
	"context"
	"fmt"
	"net/http"

	"firebase.google.com/go/v4/auth"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
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

		userRecord, err = fauth.Client.CreateUser(c, userToCreate)
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
	GenerateInvitationLink(userRecord.Email, userRecord.EmailVerified, req.BrandID)

	c.JSON(http.StatusOK, gin.H{"message": "Successfully parsed JSON", "user": userRecord})
}

// GenerateInvitationLink creates a password reset link
func GenerateInvitationLink(email string, userVerified bool, brandId string) (string, error) {
	actionCodeSettings := &auth.ActionCodeSettings{
		URL: getRedirectLink(brandId),
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
func getRedirectLink(brandId string) string {
	link := fmt.Sprintf("https://be.trendly.pro/firebase/brands/members/add?brandId=%s", brandId)
	return link
}

type FirebaseActionRequest struct {
	OobCode     string `form:"oobCode" binding:"required"`      // Out-of-band code for the action
	Mode        string `form:"mode" binding:"required"`         // Operation mode (e.g., resetPassword, verifyEmail)
	ApiKey      string `form:"apiKey" binding:"required"`       // Firebase project API key
	Lang        string `form:"lang" binding:"omitempty"`        // Language code (optional)
	ContinueUrl string `form:"continueUrl" binding:"omitempty"` // The original redirect URL
	TenantId    string `form:"tenantId" binding:"omitempty"`    // Tenant ID (optional for multi-tenancy)
}

func ValidateFirebaseCallback(c *gin.Context) {
	var req FirebaseActionRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Mode == "resetPassword" {
		// user, err := fauth.Client.VerifyPasswordResetCode(context.Background(), oobCode)
		// if err != nil {
		// 	// Handle invalid or expired code
		// }
	} else if req.Mode == "verifyEmail" {

	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request Mode", "data": req})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Success", "data": req})
}
