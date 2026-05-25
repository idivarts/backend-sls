package social_connect

import (
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/constants"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/linkedin"
)

var linkedinScopesRequired = strings.Join([]string{
	linkedin.ScopeOpenID,
	linkedin.ScopeProfile,
	linkedin.ScopeEmail,
}, " ")

// LinkedInInit redirects to LinkedIn's OAuth consent screen.
func LinkedInInit(c *gin.Context) {
	userId, _ := middlewares.GetUserId(c)
	callbackScheme := c.Query("callbackScheme")
	app := c.Query("app")
	if callbackScheme == "" || app == "" {
		c.JSON(400, gin.H{"error": "callbackScheme and app are required"})
		return
	}

	state := &OAuthState{
		UserID:         userId,
		Platform:       trendlymodels.PlatformLinkedIn,
		App:            app,
		CallbackScheme: callbackScheme,
	}
	encodedState, err := state.Encode()
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to encode state"})
		return
	}

	redirectURI := fmt.Sprintf("%s/connect/linkedin/callback", constants.TRENDLY_BE)
	authURL := fmt.Sprintf(
		"%s?response_type=code&client_id=%s&redirect_uri=%s&scope=%s&state=%s",
		linkedin.AuthURL,
		url.QueryEscape(linkedin.ClientID),
		url.QueryEscape(redirectURI),
		url.QueryEscape(linkedinScopesRequired),
		url.QueryEscape(encodedState),
	)
	c.Redirect(302, authURL)
}

// LinkedInCallback handles the OAuth callback from LinkedIn.
// GET /connect/linkedin/callback?code=...&state=...
func LinkedInCallback(c *gin.Context) {
	connectBase := constants.GetConnectFronted()

	if errParam := c.Query("error"); errParam != "" {
		log.Printf("linkedin: OAuth error: %s - %s", errParam, c.Query("error_description"))
		c.Redirect(302, CallbackErrorURL(connectBase, "linkedin", "", "", c.Query("error_description")))
		return
	}

	code := c.Query("code")
	rawState := c.Query("state")

	state, err := DecodeState(rawState)
	if err != nil {
		log.Printf("linkedin: invalid state: %v", err)
		c.Redirect(302, CallbackErrorURL(connectBase, "linkedin", "", "", "Invalid or expired connection request."))
		return
	}

	redirectURI := fmt.Sprintf("%s/connect/linkedin/callback", constants.TRENDLY_BE)

	tokens, err := linkedin.ExchangeCode(code, redirectURI)
	if err != nil {
		log.Printf("linkedin: code exchange failed: %v", err)
		c.Redirect(302, CallbackErrorURL(connectBase, "linkedin", state.CallbackScheme, state.App, "Failed to exchange authorization code."))
		return
	}

	// Fetch member profile
	profile, err := linkedin.GetMe(tokens.AccessToken)
	if err != nil {
		log.Printf("linkedin: profile fetch failed: %v", err)
		c.Redirect(302, CallbackErrorURL(connectBase, "linkedin", state.CallbackScheme, state.App, "Failed to fetch LinkedIn profile."))
		return
	}

	// LinkedIn sub is the member URN; extract the numeric ID part as username
	// e.g. "urn:li:person:abc123" → "abc123"
	parts := strings.Split(profile.Sub, ":")
	username := parts[len(parts)-1]

	socialID := trendlymodels.SocialV2ID(trendlymodels.PlatformLinkedIn, username)
	now := time.Now().Unix()

	social := &trendlymodels.SocialV2{
		ID:              socialID,
		Platform:        trendlymodels.PlatformLinkedIn,
		UserID:          state.UserID,
		Username:        username,
		DisplayName:     profile.Name,
		ProfileImageURL: profile.Picture,
		ConnectedAt:     now,
		UpdatedAt:       now,
		RawProfile: map[string]interface{}{
			"sub":        profile.Sub,
			"email":      profile.Email,
			"givenName":  profile.GivenName,
			"familyName": profile.FamilyName,
			"locale":     profile.Locale,
		},
	}

	socialPrivate := &trendlymodels.SocialV2Private{
		Platform:    trendlymodels.PlatformLinkedIn,
		AccessToken: tokens.AccessToken,
		TokenExpiry: tokens.ExpiresAt(),
		Scopes:      strings.Split(tokens.Scope, " "),
	}
	if tokens.RefreshToken != "" {
		socialPrivate.RefreshToken = tokens.RefreshToken
	}

	if err := saveSocialV2(state.UserID, socialID, social, socialPrivate); err != nil {
		log.Printf("linkedin: firestore save failed: %v", err)
		c.Redirect(302, CallbackErrorURL(connectBase, "linkedin", state.CallbackScheme, state.App, "Failed to save connection. Please try again."))
		return
	}

	c.Redirect(302, CallbackSuccessURL(connectBase, "linkedin", state.CallbackScheme, state.App))
}
