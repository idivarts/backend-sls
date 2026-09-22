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
	"github.com/idivarts/backend-sls/pkg/reddit"
)

// redditScopesRequired are the OAuth scopes requested at connect. Reddit expects
// them space-separated in the authorize URL.
var redditScopesRequired = strings.Join([]string{
	reddit.ScopeIdentity,
	reddit.ScopeSubmit,
	reddit.ScopeRead,
	reddit.ScopePrivateMessages,
	reddit.ScopeEdit,
	reddit.ScopeHistory,
}, " ")

// RedditInit redirects to Reddit's OAuth consent screen. duration=permanent is
// required to receive a refresh token (access tokens last only 1 hour).
func RedditInit(c *gin.Context) {
	if !constants.RedditEnabled {
		c.JSON(404, gin.H{"error": "reddit integration is not enabled"})
		return
	}
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
		Platform:       trendlymodels.PlatformReddit,
		App:            app,
		CallbackScheme: callbackScheme,
		BrandID:        brandId,
	}
	encodedState, err := state.Encode()
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to encode state"})
		return
	}

	redirectURI := fmt.Sprintf("%s/connect/reddit/callback", constants.GetTrendlyBE())
	authURL := fmt.Sprintf(
		"%s?client_id=%s&response_type=code&state=%s&redirect_uri=%s&duration=permanent&scope=%s",
		reddit.AuthURL,
		url.QueryEscape(reddit.ClientID),
		url.QueryEscape(encodedState),
		url.QueryEscape(redirectURI),
		url.QueryEscape(redditScopesRequired),
	)
	c.Redirect(302, authURL)
}

// RedditCallback handles the OAuth callback from Reddit.
// GET /connect/reddit/callback?code=...&state=...
func RedditCallback(c *gin.Context) {
	connectBase := constants.GetConnectFronted()

	if errParam := c.Query("error"); errParam != "" {
		log.Printf("reddit: OAuth error: %s", errParam)
		c.Redirect(302, CallbackErrorURL(connectBase, "reddit", "", "", "Authorization was denied or cancelled."))
		return
	}

	code := c.Query("code")
	rawState := c.Query("state")

	state, err := DecodeState(rawState)
	if err != nil {
		log.Printf("reddit: invalid state: %v", err)
		c.Redirect(302, CallbackErrorURL(connectBase, "reddit", "", "", "Invalid or expired connection request."))
		return
	}

	redirectURI := fmt.Sprintf("%s/connect/reddit/callback", constants.GetTrendlyBE())

	tokens, err := reddit.ExchangeCode(code, redirectURI)
	if err != nil {
		log.Printf("reddit: code exchange failed: %v", err)
		c.Redirect(302, CallbackErrorURL(connectBase, "reddit", state.CallbackScheme, state.App, "Failed to exchange authorization code."))
		return
	}

	user, err := reddit.GetMe(tokens.AccessToken)
	if err != nil {
		log.Printf("reddit: profile fetch failed: %v", err)
		c.Redirect(302, CallbackErrorURL(connectBase, "reddit", state.CallbackScheme, state.App, "Failed to fetch Reddit profile."))
		return
	}

	socialID := trendlymodels.SocialAccountID(trendlymodels.PlatformReddit, user.Name)
	now := time.Now().Unix()

	social := &trendlymodels.SocialAccount{
		ID:                socialID,
		Platform:          trendlymodels.PlatformReddit,
		UserID:            state.UserID,
		PlatformAccountID: "t2_" + user.ID, // Reddit account fullname
		Username:          user.Name,
		DisplayName:       firstNonEmptyStr(user.Subreddit.Title, user.Name),
		ProfileImageURL:   user.AvatarURL(),
		Bio:               user.Subreddit.PublicDescription,
		ProfileURL:        "https://www.reddit.com/user/" + user.Name,
		// Reddit has no "followers" concept for accounts; karma is the closest
		// public signal and is surfaced via rawProfile / derived analytics.
		ConnectedAt: now,
		UpdatedAt:   now,
		RawProfile: map[string]interface{}{
			"id":           user.ID,
			"totalKarma":   user.TotalKarma,
			"linkKarma":    user.LinkKarma,
			"commentKarma": user.CommentKarma,
		},
	}

	socialToken := &trendlymodels.SocialToken{
		Platform:     trendlymodels.PlatformReddit,
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
		log.Printf("reddit: firestore save failed: %v", saveErr)
		c.Redirect(302, CallbackErrorURL(connectBase, "reddit", state.CallbackScheme, state.App, "Failed to save connection. Please try again."))
		return
	}

	c.Redirect(302, CallbackSuccessURL(connectBase, "reddit", state.CallbackScheme, state.App))
}

// firstNonEmptyStr returns the first non-empty string from the arguments.
func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
