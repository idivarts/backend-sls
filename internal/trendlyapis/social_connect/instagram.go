package social_connect

import (
	"context"
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
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"github.com/idivarts/backend-sls/pkg/instagram"
)

// InstagramInit redirects the browser to Instagram's OAuth consent screen.
// Query params (from connect portal):
//   - token        Firebase JWT (validated by middleware before this handler runs)
//   - callbackScheme  e.g. trn-users or trn-brands
//   - app          "users" | "brands"
//   - userId       injected by middleware
func InstagramInit(c *gin.Context) {
	userId, _ := middlewares.GetUserId(c)
	callbackScheme := c.Query("callbackScheme")
	app := c.Query("app")
	if callbackScheme == "" || app == "" {
		c.JSON(400, gin.H{"error": "callbackScheme and app are required"})
		return
	}

	state := &OAuthState{
		UserID:         userId,
		Platform:       trendlymodels.PlatformInstagram,
		App:            app,
		CallbackScheme: callbackScheme,
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

	// Instagram sometimes sends error instead of code
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

	// Exchange code for short-lived token
	shortToken, err := instagram.GetAccessTokenFromCode(code, redirectURI)
	if err != nil {
		log.Printf("instagram: code exchange failed: %v", err)
		c.Redirect(302, CallbackErrorURL(connectBase, "instagram", state.CallbackScheme, state.App, "Failed to exchange authorization code."))
		return
	}

	// Exchange for long-lived token (valid 60 days)
	longToken, err := instagram.GetLongLivedAccessToken(shortToken.AccessToken)
	if err != nil {
		log.Printf("instagram: long-lived token exchange failed: %v", err)
		c.Redirect(302, CallbackErrorURL(connectBase, "instagram", state.CallbackScheme, state.App, "Failed to obtain long-lived token."))
		return
	}

	// Fetch profile
	instaProfile, err := instagram.GetInstagram("me", longToken.AccessToken)
	if err != nil {
		log.Printf("instagram: profile fetch failed: %v", err)
		c.Redirect(302, CallbackErrorURL(connectBase, "instagram", state.CallbackScheme, state.App, "Failed to fetch Instagram profile."))
		return
	}

	username := instaProfile.Username
	socialID := trendlymodels.SocialV2ID(trendlymodels.PlatformInstagram, username)
	now := time.Now().Unix()

	// Expiry: Instagram long-lived tokens last 60 days
	tokenExpiry := now + longToken.ExpiresIn

	social := &trendlymodels.SocialV2{
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

	socialPrivate := &trendlymodels.SocialV2Private{
		Platform:    trendlymodels.PlatformInstagram,
		AccessToken: longToken.AccessToken,
		TokenExpiry: tokenExpiry,
		Scopes:      shortToken.Permissions,
	}

	if err := saveSocialV2(state.UserID, socialID, social, socialPrivate); err != nil {
		log.Printf("instagram: firestore save failed: %v", err)
		c.Redirect(302, CallbackErrorURL(connectBase, "instagram", state.CallbackScheme, state.App, "Failed to save connection. Please try again."))
		return
	}

	c.Redirect(302, CallbackSuccessURL(connectBase, "instagram", state.CallbackScheme, state.App))
}

// saveSocialV2 writes public and private social docs to Firestore in parallel.
func saveSocialV2(userID, socialID string, social *trendlymodels.SocialV2, priv *trendlymodels.SocialV2Private) error {
	ctx := context.Background()

	pubRef := firestoredb.Client.Collection(fmt.Sprintf("users/%s/socialsV2", userID)).Doc(socialID)
	privRef := firestoredb.Client.Collection(fmt.Sprintf("users/%s/socialsV2Private", userID)).Doc(socialID)

	// Use a batch write for atomicity
	batch := firestoredb.Client.Batch()
	batch.Set(pubRef, social)
	batch.Set(privRef, priv)

	if _, err := batch.Commit(ctx); err != nil {
		return fmt.Errorf("saveSocialV2: batch commit failed: %w", err)
	}
	return nil
}
