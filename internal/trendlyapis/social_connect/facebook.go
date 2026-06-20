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

	redirectURI := fmt.Sprintf("%s/connect/facebook/callback", constants.GetTrendlyBE())
	authURL := fmt.Sprintf(
		"https://www.facebook.com/dialog/oauth?client_id=%s&redirect_uri=%s&scope=%s&state=%s&response_type=code",
		messenger.ClientID,
		url.QueryEscape(redirectURI),
		url.QueryEscape("pages_show_list,pages_read_engagement,pages_messaging,pages_manage_engagement,pages_manage_metadata,pages_manage_posts"),
		// ,instagram_basic,instagram_manage_insights,instagram_manage_messages,instagram_manage_comments
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

	redirectURI := fmt.Sprintf("%s/connect/facebook/callback", constants.GetTrendlyBE())

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

	// We store one SocialAccount per managed Page (not the personal FB user),
	// because the inbox needs Page access tokens to read/reply to Messenger and
	// Page comments, and the page-linked IG Business Account for IG messaging.
	_, accounts, err := messenger.GetMyFacebook(fbUserID, longToken.AccessToken)
	if err != nil {
		log.Printf("facebook: profile fetch failed: %v", err)
		c.Redirect(302, CallbackErrorURL(connectBase, "facebook", state.CallbackScheme, state.App, "Failed to fetch Facebook profile."))
		return
	}

	var pages []messenger.FacebookProfile
	if accounts != nil {
		pages = accounts.Accounts.Data
	}
	if len(pages) == 0 {
		log.Printf("facebook: no managed pages for user %s", fbUserID)
		c.Redirect(302, CallbackErrorURL(connectBase, "facebook", state.CallbackScheme, state.App, "No Facebook Pages found. Connect a Page you manage to use the inbox."))
		return
	}

	now := time.Now().Unix()
	savedCount := 0

	for i := range pages {
		page := pages[i]
		if page.ID == "" || page.AccessToken == "" {
			continue
		}

		pageSocialID := trendlymodels.SocialAccountID(trendlymodels.PlatformFacebook, page.ID)
		igBusinessID := ""
		if page.InstagramBusinessAccount != nil {
			igBusinessID = page.InstagramBusinessAccount.ID
		}

		social := &trendlymodels.SocialAccount{
			ID:                  pageSocialID,
			Platform:            trendlymodels.PlatformFacebook,
			UserID:              state.UserID,
			PlatformAccountID:   page.ID,
			InstagramBusinessID: igBusinessID,
			Username:            page.ID,
			DisplayName:         page.Name,
			ProfileImageURL:     page.Picture.Data.URL,
			FollowerCount:       int64(page.FollowersCount),
			ConnectedAt:         now,
			UpdatedAt:           now,
			RawProfile: map[string]interface{}{
				"id":                       page.ID,
				"name":                     page.Name,
				"fbUserId":                 fbUserID,
				"instagramBusinessAccount": igBusinessID,
			},
		}

		socialToken := &trendlymodels.SocialToken{
			Platform:    trendlymodels.PlatformFacebook,
			AccessToken: page.AccessToken, // PAGE token (not the user token)
			TokenExpiry: 0,                // page tokens from a long-lived user token don't expire
		}

		var saveErr error
		if state.BrandID != "" {
			saveErr = trendlymodels.SaveBrandSocialAccount(state.BrandID, social, socialToken)
		} else {
			saveErr = trendlymodels.SaveSocialAccount(state.UserID, social, socialToken)
		}
		if saveErr != nil {
			log.Printf("facebook: firestore save failed for page %s: %v", page.ID, saveErr)
			continue
		}
		savedCount++

		// Index the Page id, and the linked IG Business Account id — page-linked
		// IG message/comment webhooks arrive under the IG id but are served with
		// this Page's token, so both resolve to the same social account.
		upsertSocialIndex(page.ID, trendlymodels.PlatformFacebook, state, pageSocialID, now)
		if igBusinessID != "" {
			upsertSocialIndex(igBusinessID, trendlymodels.PlatformInstagram, state, pageSocialID, now)
		}

		// Subscribe the Page to inbox webhooks (DMs + comments). Best-effort:
		// real-time delivery degrades on failure, the connection still succeeds.
		if err := messenger.SubscribeApp(true, page.AccessToken); err != nil {
			log.Printf("facebook: webhook subscribe failed for page %s: %v", page.ID, err)
		}
	}

	if savedCount == 0 {
		c.Redirect(302, CallbackErrorURL(connectBase, "facebook", state.CallbackScheme, state.App, "Failed to save connection. Please try again."))
		return
	}

	c.Redirect(302, CallbackSuccessURL(connectBase, "facebook", state.CallbackScheme, state.App))
}
