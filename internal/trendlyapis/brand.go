package trendlyapis

import (
	"context"
	"net/http"

	"firebase.google.com/go/auth"
	"github.com/gin-gonic/gin"
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
	// fauth.Client.EmailSignInLink()

	c.JSON(http.StatusOK, gin.H{"message": "Successfully parsed JSON", "user": userRecord})
}

// GenerateInvitationLink creates a password reset link
func GenerateInvitationLink(email string) (string, error) {
	actionCodeSettings := &auth.ActionCodeSettings{
		URL:             "https://yourapp.com/complete-registration",
		HandleCodeInApp: true,
	}
	link, err := fauth.Client.PasswordResetLinkWithSettings(context.Background(), email, actionCodeSettings)
	if err != nil {
		return "", err
	}
	return link, nil
}
