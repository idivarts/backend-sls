package social_connect

import (
	"fmt"
	"log"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/constants"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/messenger"
)

// FacebookInit redirects to Facebook's OAuth consent screen.
func FacebookInit(c *gin.Context) {
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
		Platform:       trendlymodels.PlatformFacebook,
		App:            app,
		CallbackScheme: callbackScheme,
		BrandID:        brandId,
	}
	encodedState, err := state.Encode()
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to encode state"})
		return
	}

	redirectURI := fmt.Sprintf("%s/connect/facebook/callback", constants.TRENDLY_BE)
	authURL := fmt.Sprintf(
		"https://www.facebook.com/dialog/oauth?client_id=%s&redirect_uri=%s&scope=%s&state=%s&response_type=code",
		messenger.ClientID,
		url.QueryEscape(redirectURI),
		url.QueryEscape("pages_show_list,instagram_basic,instagram_manage_insights,pages_read_engagement"),
		url.QueryEscape(encodedState),
	)
	c.Redirect(302, authURL)
}

// FacebookCallback handles the OAuth callback from Facebook.
// GET /connect/facebook/callback?code=...&state=...
func FacebookCallback(c *gin.Context) {
	connectBase := constants.GetConnectFronted()

	if errParam := c.Query("error"); errParam != "" {
		log.Printf("facebook: OAuth error: %s - %s", errParam, c.Query("error_description"))
		c.Redirect(302, CallbackErrorURL(connectBase, "facebook", "", "", c.Query("error_description")))
		return
	}

	code := c.Query("code")
	rawState := c.Query("state")

	state, err := DecodeState(rawState)
	if err != nil {
		log.Printf("facebook: invalid state: %v", err)
		c.Redirect(302, CallbackErrorURL(connectBase, "facebook", "", "", "Invalid or expired connection request."))
		return
	}

	redirectURI := fmt.Sprintf("%s/connect/facebook/callback", constants.TRENDLY_BE)

	shortToken, err := messenger.GetAccessTokenFromCode(code, redirectURI)
	if err != nil {
		log.Printf("facebook: code exchange failed: %v", err)
		c.Redirect(302, CallbackErrorURL(connectBase, "facebook", state.CallbackScheme, state.App, "Failed to exchange authorization code."))
		return
	}

	longToken, err := messenger.GetLongLivedAccessToken(shortToken.AccessToken)
	if err != nil {
		log.Printf("facebook: long-lived token failed: %v", err)
		c.Redirect(302, CallbackErrorURL(connectBase, "facebook", state.CallbackScheme, state.App, "Failed to obtain long-lived token."))
		return
	}

	fbUserID, err := messenger.GetMeID(longToken.AccessToken)
	if err != nil {
		log.Printf("facebook: /me fetch failed: %v", err)
		c.Redirect(302, CallbackErrorURL(connectBase, "facebook", state.CallbackScheme, state.App, "Failed to fetch Facebook profile."))
		return
	}

	fbProfile, _, err := messenger.GetMyFacebook(fbUserID, longToken.AccessToken)
	if err != nil {
		log.Printf("facebook: profile fetch failed: %v", err)
		c.Redirect(302, CallbackErrorURL(connectBase, "facebook", state.CallbackScheme, state.App, "Failed to fetch Facebook profile."))
		return
	}

	socialID := trendlymodels.SocialAccountID(trendlymodels.PlatformFacebook, fbUserID)
	now := time.Now().Unix()

	social := &trendlymodels.SocialAccount{
		ID:              socialID,
		Platform:        trendlymodels.PlatformFacebook,
		UserID:          state.UserID,
		Username:        fbUserID,
		DisplayName:     fbProfile.Name,
		ProfileImageURL: fbProfile.Picture.Data.URL,
		FollowerCount:   int64(fbProfile.FollowersCount),
		ConnectedAt:     now,
		UpdatedAt:       now,
		RawProfile: map[string]interface{}{
			"id":   fbUserID,
			"name": fbProfile.Name,
		},
	}

	socialToken := &trendlymodels.SocialToken{
		Platform:    trendlymodels.PlatformFacebook,
		AccessToken: longToken.AccessToken,
		TokenExpiry: now + longToken.ExpiresIn,
	}

	var saveErr error
	if state.BrandID != "" {
		saveErr = trendlymodels.SaveBrandSocialAccount(state.BrandID, social, socialToken)
	} else {
		saveErr = trendlymodels.SaveSocialAccount(state.UserID, social, socialToken)
	}
	if saveErr != nil {
		log.Printf("facebook: firestore save failed: %v", saveErr)
		c.Redirect(302, CallbackErrorURL(connectBase, "facebook", state.CallbackScheme, state.App, "Failed to save connection. Please try again."))
		return
	}

	c.Redirect(302, CallbackSuccessURL(connectBase, "facebook", state.CallbackScheme, state.App))
}
