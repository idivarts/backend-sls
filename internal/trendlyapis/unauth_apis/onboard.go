package trendlyunauth

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"firebase.google.com/go/v4/auth"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/pkg/analytics"
	"github.com/idivarts/backend-sls/pkg/firebase/fauth"
	"github.com/idivarts/backend-sls/pkg/myemail"

	"github.com/idivarts/backend-sls/internal/constants"
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

type CheckEmailRequest struct {
	Email string `json:"email" binding:"required"`
}

// Signup creates a new Firebase Auth account with email and password,
// then sends a custom verification email (delivery is Amazon SES v2 — see
// pkg/myemail; the comment here used to say SendGrid).
//
// Emits E3 of the email signup funnel. E1/E2 are reported by the app, which
// cannot see this step: a 2xx below means "account created AND mail
// dispatched", so the app has no way to report the send on its own.
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

	// Build the verification link pointing to our email-redirection endpoint.
	// GetTrendlyBE() is stage-aware (adds the "/dev" API Gateway stage in dev).
	verificationLink := fmt.Sprintf("%s/onboard/email-redirection?token=%s", constants.GetTrendlyBE(), url.QueryEscape(token))

	// Send custom verification email via SendGrid
	data := map[string]interface{}{
		"ManagerName":      req.Name,
		"VerificationLink": verificationLink,
	}
	err = myemail.SendCustomHTMLEmail(req.Email, templates.EmailVerification, templates.SubjectEmailVerification, data)
	if err != nil {
		// The account exists but the mail did not go out, which is the one
		// failure mode the app cannot see: it only learns the request failed,
		// not that a user is now stranded with an unverifiable account.
		analytics.Capture(c.Request.Context(), userRecord.UID, "signup_verification_email_failed", map[string]interface{}{
			"reason": err.Error(),
		})
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Error sending verification email"})
		return
	}

	// E3 of the email funnel. Keyed on the Firebase uid so it stitches to the
	// app's own identify() for the same user — the email itself is deliberately
	// not sent as a property.
	//
	// Synchronous, not CaptureAsync. The async version lost this event outright
	// on dev: the client fired signup_email_submitted and signup_email_registered
	// and the mail went out, but nothing arrived from here. A goroutine is not
	// guaranteed to run once the handler returns — Lambda freezes the execution
	// environment on response, which kills the in-flight POST. The package doc
	// warned about this; the signup path turns out to be exactly the case it
	// warned about.
	//
	// The cost is one short POST before responding, bounded by the client's 5s
	// timeout and swallowed on failure, which is the right trade for the event
	// that opens the funnel.
	analytics.Capture(c.Request.Context(), userRecord.UID, "signup_verification_email_sent", nil)

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

	// E4 of the email funnel, and the step only the backend can see: the link
	// is consumed HERE, before any redirect, so a client-side version of this
	// event would miss everyone who opens the mail in a webview or on a device
	// where the app intercepts the link.
	//
	// Captured synchronously rather than async: a redirect ends the handler
	// immediately, and a goroutine racing a frozen Lambda would lose the event
	// that closes the funnel. One short POST is worth the certainty here.
	analytics.Capture(c.Request.Context(), uid, "signup_email_verified", nil)

	// Redirect to Trendly Brands login page with email pre-filled
	c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s%s?email=%s", constants.GetBrandsFronted(), os.Getenv("BRAND_LOGIN_URL"), url.QueryEscape(uRecord.Email)))
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
		URL:             fmt.Sprintf("%s%s", constants.GetBrandsFronted(), os.Getenv("BRAND_LOGIN_URL")),
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

// CheckEmail reports whether a brand-manager account already exists for the given
// email. Firebase Auth is the source of truth (a manager Firestore doc is only
// created on first successful sign-in, so a signed-up-but-never-logged-in account
// would be missed by a Firestore query). The pre-signin screen uses this to route
// the user to login (exists) vs create-new-account (does not exist).
func CheckEmail(c *gin.Context) {
	var req CheckEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err := fauth.Client.GetUserByEmail(context.Background(), req.Email)
	if err != nil {
		if auth.IsUserNotFound(err) {
			c.JSON(http.StatusOK, gin.H{"exists": false})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Error checking email"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"exists": true})
}
