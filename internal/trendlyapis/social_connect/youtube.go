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
	"github.com/idivarts/backend-sls/pkg/youtube"
)

var youtubeScopesRequired = strings.Join([]string{
	youtube.ScopeYouTubeReadonly,
	youtube.ScopeYTAnalytics,
	youtube.ScopeUserInfoProfile,
	youtube.ScopeUserInfoEmail,
}, " ")

// YouTubeInit redirects to Google's OAuth consent screen.
func YouTubeInit(c *gin.Context) {
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
		Platform:       trendlymodels.PlatformYouTube,
		App:            app,
		CallbackScheme: callbackScheme,
		BrandID:        brandId,
	}
	encodedState, err := state.Encode()
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to encode state"})
		return
	}

	redirectURI := fmt.Sprintf("%s/connect/youtube/callback", constants.TRENDLY_BE)
	authURL := fmt.Sprintf(
		"%s?client_id=%s&redirect_uri=%s&response_type=code&scope=%s&state=%s"+
			"&access_type=offline&prompt=consent",
		youtube.AuthURL,
		url.QueryEscape(youtube.ClientID),
		url.QueryEscape(redirectURI),
		url.QueryEscape(youtubeScopesRequired),
		url.QueryEscape(encodedState),
	)
	c.Redirect(302, authURL)
}

// YouTubeCallback handles the OAuth callback from Google.
// GET /connect/youtube/callback?code=...&state=...
func YouTubeCallback(c *gin.Context) {
	connectBase := constants.GetConnectFronted()

	if errParam := c.Query("error"); errParam != "" {
		log.Printf("youtube: OAuth error: %s", errParam)
		c.Redirect(302, CallbackErrorURL(connectBase, "youtube", "", "", "Authorization was denied or cancelled."))
		return
	}

	code := c.Query("code")
	rawState := c.Query("state")

	state, err := DecodeState(rawState)
	if err != nil {
		log.Printf("youtube: invalid state: %v", err)
		c.Redirect(302, CallbackErrorURL(connectBase, "youtube", "", "", "Invalid or expired connection request."))
		return
	}

	redirectURI := fmt.Sprintf("%s/connect/youtube/callback", constants.TRENDLY_BE)

	tokens, err := youtube.ExchangeCode(code, redirectURI)
	if err != nil {
		log.Printf("youtube: code exchange failed: %v", err)
		c.Redirect(302, CallbackErrorURL(connectBase, "youtube", state.CallbackScheme, state.App, "Failed to exchange authorization code."))
		return
	}

	channel, err := youtube.GetMyChannel(tokens.AccessToken)
	if err != nil {
		log.Printf("youtube: channel fetch failed: %v", err)
		c.Redirect(302, CallbackErrorURL(connectBase, "youtube", state.CallbackScheme, state.App, "Failed to fetch YouTube channel."))
		return
	}

	handle := channel.Snippet.CustomURL // e.g. "@handle"
	if handle == "" {
		handle = channel.ID // fallback to channel ID
	}
	handle = strings.TrimPrefix(handle, "@")

	socialID := trendlymodels.SocialAccountID(trendlymodels.PlatformYouTube, handle)
	now := time.Now().Unix()

	social := &trendlymodels.SocialAccount{
		ID:              socialID,
		Platform:        trendlymodels.PlatformYouTube,
		UserID:          state.UserID,
		Username:        handle,
		DisplayName:     channel.Snippet.Title,
		ProfileImageURL: channel.Snippet.Thumbnails.High.URL,
		Bio:             channel.Snippet.Description,
		ProfileURL:      "https://www.youtube.com/@" + handle,
		FollowerCount:   parseCount(channel.Stats.SubscriberCount),
		MediaCount:      parseCount(channel.Stats.VideoCount),
		ConnectedAt:     now,
		UpdatedAt:       now,
		RawProfile: map[string]interface{}{
			"channelId":  channel.ID,
			"customUrl":  channel.Snippet.CustomURL,
			"country":    channel.Snippet.Country,
			"viewCount":  channel.Stats.ViewCount,
			"videoCount": channel.Stats.VideoCount,
		},
	}

	socialToken := &trendlymodels.SocialToken{
		Platform:     trendlymodels.PlatformYouTube,
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		TokenExpiry:  tokens.ExpiresAt(),
		Scopes:       strings.Split(tokens.Scope, " "),
	}

	var saveErr error
	if state.BrandID != "" {
		saveErr = trendlymodels.SaveBrandSocialAccount(state.BrandID, social, socialToken)
	} else {
		saveErr = trendlymodels.SaveSocialAccount(state.UserID, social, socialToken)
	}
	if saveErr != nil {
		log.Printf("youtube: firestore save failed: %v", saveErr)
		c.Redirect(302, CallbackErrorURL(connectBase, "youtube", state.CallbackScheme, state.App, "Failed to save connection. Please try again."))
		return
	}

	c.Redirect(302, CallbackSuccessURL(connectBase, "youtube", state.CallbackScheme, state.App))
}

// parseCount converts a string number to int64, returning 0 on error.
func parseCount(s string) int64 {
	if s == "" {
		return 0
	}
	var n int64
	fmt.Sscanf(s, "%d", &n)
	return n
}
