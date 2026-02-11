package trendlyunauth

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"firebase.google.com/go/v4/auth"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/pkg/firebase/fauth"
	"github.com/idivarts/backend-sls/pkg/myemail"
	"github.com/idivarts/backend-sls/pkg/myutil"

	myjwt "github.com/idivarts/backend-sls/internal/trendlyapis/jwt"
	templates "github.com/idivarts/backend-sls/templates"
)

type SignupRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
	Name     string `json:"name" binding:"required"`
}

type ResetPasswordRequest struct {
	Email string `json:"email" binding:"required"`
}

// Signup creates a new Firebase Auth account with email and password,
// then sends a custom verification email via SendGrid.
func Signup(c *gin.Context) {
	var req SignupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Create user in Firebase Auth with email verified set to false
	userToCreate := (&auth.UserToCreate{}).
		Email(req.Email).
		Password(req.Password).
		DisplayName(req.Name).
		EmailVerified(false)

	userRecord, err := fauth.Client.CreateUser(context.Background(), userToCreate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error creating user account"})
		return
	}

	// Generate a JWT token encoding the user's UID for email verification
	token, err := myjwt.EncodeUID(userRecord.UID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Error generating verification token"})
		return
	}

	dev := ""
	if myutil.IsDevEnvironment() {
		dev = "/dev"
	}
	// Build the verification link pointing to our email-redirection endpoint
	verificationLink := fmt.Sprintf("%s%s/onboard/email-redirection?token=%s", os.Getenv("SELF_BASE_URL"), dev, url.QueryEscape(token))

	// Send custom verification email via SendGrid
	data := map[string]interface{}{
		"ManagerName":      req.Name,
		"VerificationLink": verificationLink,
	}
	err = myemail.SendCustomHTMLEmail(req.Email, templates.EmailVerification, templates.SubjectEmailVerification, data)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Error sending verification email"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Account created successfully. Please check your email to verify your account."})
}

// EmailRedirection handles the verification link clicked from the signup email.
// It marks the account as verified and redirects the user to the Trendly Brands login page.
func EmailRedirection(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Token is required"})
		return
	}

	// Decode the JWT to get the user's UID
	uid, err := myjwt.DecodeUID(token)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid or expired token"})
		return
	}

	// Mark the user's email as verified in Firebase Auth
	userToUpdate := (&auth.UserToUpdate{}).EmailVerified(true)
	uRecord, err := fauth.Client.UpdateUser(context.Background(), uid, userToUpdate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Error verifying account"})
		return
	}

	// Redirect to Trendly Brands login page with email pre-filled
	c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s?email=%s", os.Getenv("BRAND_LOGIN_URL"), url.QueryEscape(uRecord.Email)))
}

// ResetPassword takes an email and sends a custom password reset email via SendGrid
// with a Firebase-generated password reset link.
func ResetPassword(c *gin.Context) {
	var req ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify the user exists and get their display name
	userRecord, err := fauth.Client.GetUserByEmail(context.Background(), req.Email)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "User not found"})
		return
	}

	// Generate a Firebase password reset link
	actionCodeSettings := &auth.ActionCodeSettings{
		URL:             os.Getenv("BRAND_LOGIN_URL"),
		HandleCodeInApp: true,
	}
	resetLink, err := fauth.Client.PasswordResetLinkWithSettings(context.Background(), req.Email, actionCodeSettings)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Error generating reset link"})
		return
	}

	// Send custom password reset email via SendGrid
	userName := userRecord.DisplayName
	if userName == "" {
		userName = req.Email
	}
	data := map[string]interface{}{
		"UserName":  userName,
		"ResetLink": resetLink,
	}
	err = myemail.SendCustomHTMLEmail(req.Email, templates.PasswordReset, templates.SubjectPasswordReset, data)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Error sending reset email"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Password reset email sent successfully. Please check your inbox."})
}
