package social_connect

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/constants"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/instagram"
)

// InstagramInit redirects the browser to Instagram's OAuth consent screen.
// Query params (from connect portal):
//   - token           Firebase JWT (validated by middleware before this handler runs)
//   - callbackScheme  e.g. trn-users or trn-brands
//   - app             "users" | "brands"
func InstagramInit(c *gin.Context) {
	userId, _ := middlewares.GetUserId(c)
	callbackScheme := c.Query("callbackScheme")
	app := c.Query("app")
	if callbackScheme == "" || app == "" {
		c.JSON(400, gin.H{"error": "callbackScheme and app are required"})
		return
	}

	brandId := c.Query("brandId")
	state := &OAuthState{
		UserID:         userId,
		Platform:       trendlymodels.PlatformInstagram,
		App:            app,
		CallbackScheme: callbackScheme,
		BrandID:        brandId,
	}
	encodedState, err := state.Encode()
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to encode state"})
		return
	}

	clientId := os.Getenv("INSTA_CLIENT_ID")
	if clientId == "" {
		c.JSON(500, gin.H{"error": "Instagram client ID not configured"})
		return
	}

	redirectURI := fmt.Sprintf("%s/connect/instagram/callback", constants.TRENDLY_BE)
	authURL := fmt.Sprintf(
		"https://www.instagram.com/oauth/authorize?enable_fb_login=1&force_authentication=0"+
			"&client_id=%s&redirect_uri=%s&response_type=code"+
			"&scope=instagram_business_basic,instagram_business_manage_insights"+
			"&state=%s",
		clientId, url.QueryEscape(redirectURI), url.QueryEscape(encodedState),
	)
	c.Redirect(302, authURL)
}

// InstagramCallback handles the OAuth callback from Instagram.
// GET /connect/instagram/callback?code=...&state=...
func InstagramCallback(c *gin.Context) {
	connectBase := constants.GetConnectFronted()

	code := c.Query("code")
	rawState := c.Query("state")

	if errParam := c.Query("error"); errParam != "" {
		log.Printf("instagram: OAuth error: %s - %s", errParam, c.Query("error_reason"))
		c.Redirect(302, CallbackErrorURL(connectBase, "instagram", "", "", c.Query("error_description")))
		return
	}

	state, err := DecodeState(rawState)
	if err != nil {
		log.Printf("instagram: invalid state: %v", err)
		c.Redirect(302, CallbackErrorURL(connectBase, "instagram", "", "", "Invalid or expired connection request."))
		return
	}

	redirectURI := fmt.Sprintf("%s/connect/instagram/callback", constants.TRENDLY_BE)

	shortToken, err := instagram.GetAccessTokenFromCode(code, redirectURI)
	if err != nil {
		log.Printf("instagram: code exchange failed: %v", err)
		c.Redirect(302, CallbackErrorURL(connectBase, "instagram", state.CallbackScheme, state.App, "Failed to exchange authorization code."))
		return
	}

	longToken, err := instagram.GetLongLivedAccessToken(shortToken.AccessToken)
	if err != nil {
		log.Printf("instagram: long-lived token exchange failed: %v", err)
		c.Redirect(302, CallbackErrorURL(connectBase, "instagram", state.CallbackScheme, state.App, "Failed to obtain long-lived token."))
		return
	}

	instaProfile, err := instagram.GetInstagram("me", longToken.AccessToken)
	if err != nil {
		log.Printf("instagram: profile fetch failed: %v", err)
		c.Redirect(302, CallbackErrorURL(connectBase, "instagram", state.CallbackScheme, state.App, "Failed to fetch Instagram profile."))
		return
	}

	username := instaProfile.Username
	socialID := trendlymodels.SocialAccountID(trendlymodels.PlatformInstagram, username)
	now := time.Now().Unix()

	social := &trendlymodels.SocialAccount{
		ID:              socialID,
		Platform:        trendlymodels.PlatformInstagram,
		UserID:          state.UserID,
		Username:        username,
		DisplayName:     instaProfile.Name,
		ProfileImageURL: instaProfile.ProfilePictureURL,
		FollowerCount:   int64(instaProfile.FollowersCount),
		FollowingCount:  int64(instaProfile.FollowsCount),
		MediaCount:      int64(instaProfile.MediaCount),
		ConnectedAt:     now,
		UpdatedAt:       now,
		RawProfile: map[string]interface{}{
			"id": strconv.FormatInt(shortToken.UserID, 10),
		},
	}

	socialToken := &trendlymodels.SocialToken{
		Platform:    trendlymodels.PlatformInstagram,
		AccessToken: longToken.AccessToken,
		TokenExpiry: now + longToken.ExpiresIn,
		Scopes:      shortToken.Permissions,
	}

	var saveErr error
	if state.BrandID != "" {
		social.UserID = state.UserID // keep userId for audit; Firestore path uses brandId
		saveErr = trendlymodels.SaveBrandSocialAccount(state.BrandID, social, socialToken)
	} else {
		saveErr = trendlymodels.SaveSocialAccount(state.UserID, social, socialToken)
	}
	if saveErr != nil {
		log.Printf("instagram: firestore save failed: %v", saveErr)
		c.Redirect(302, CallbackErrorURL(connectBase, "instagram", state.CallbackScheme, state.App, "Failed to save connection. Please try again."))
		return
	}

	c.Redirect(302, CallbackSuccessURL(connectBase, "instagram", state.CallbackScheme, state.App))
}
