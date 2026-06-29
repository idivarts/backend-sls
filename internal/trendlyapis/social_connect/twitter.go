package social_connect

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/constants"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/twitter"
)

var twitterScopesRequired = strings.Join([]string{
	twitter.ScopeTweetRead,
	twitter.ScopeUsersRead,
	twitter.ScopeOfflineAccess,
	// Write + DM scopes power posting, the replies/mentions media inbox and the
	// DM messaging inbox. They require the X app to be set to "Read and write and
	// Direct message" — see docs/social-expansion-dashboard-setup.md §1.
	twitter.ScopeTweetWrite,
	twitter.ScopeMediaWrite,
	twitter.ScopeDMRead,
	twitter.ScopeDMWrite,
}, " ")

// TwitterInit redirects to Twitter's OAuth 2.0 PKCE consent screen.
// The PKCE code_verifier is embedded in the state so it survives the redirect.
func TwitterInit(c *gin.Context) {
	userId, _ := middlewares.GetUserId(c)
	callbackScheme := c.Query("callbackScheme")
	app := c.Query("app")
	if callbackScheme == "" || app == "" {
		c.JSON(400, gin.H{"error": "callbackScheme and app are required"})
		return
	}

	codeVerifier, codeChallenge, err := generatePKCE()
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to generate PKCE"})
		return
	}

	brandId := c.Query("brandId")
	state := &OAuthState{
		UserID:         userId,
		Platform:       trendlymodels.PlatformTwitter,
		App:            app,
		CallbackScheme: callbackScheme,
		BrandID:        brandId,
		CodeVerifier:   codeVerifier,
	}
	encodedState, err := state.Encode()
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to encode state"})
		return
	}

	redirectURI := fmt.Sprintf("%s/connect/twitter/callback", constants.GetTrendlyBE())
	authURL := fmt.Sprintf(
		"%s?response_type=code&client_id=%s&redirect_uri=%s&scope=%s"+
			"&state=%s&code_challenge=%s&code_challenge_method=S256",
		twitter.AuthURL,
		url.QueryEscape(twitter.ClientID),
		url.QueryEscape(redirectURI),
		url.QueryEscape(twitterScopesRequired),
		url.QueryEscape(encodedState),
		url.QueryEscape(codeChallenge),
	)
	c.Redirect(302, authURL)
}

// TwitterCallback handles the OAuth 2.0 PKCE callback from Twitter/X.
// GET /connect/twitter/callback?code=...&state=...
func TwitterCallback(c *gin.Context) {
	connectBase := constants.GetConnectFronted()

	if errParam := c.Query("error"); errParam != "" {
		log.Printf("twitter: OAuth error: %s - %s", errParam, c.Query("error_description"))
		c.Redirect(302, CallbackErrorURL(connectBase, "twitter", "", "", c.Query("error_description")))
		return
	}

	code := c.Query("code")
	rawState := c.Query("state")

	state, err := DecodeState(rawState)
	if err != nil {
		log.Printf("twitter: invalid state: %v", err)
		c.Redirect(302, CallbackErrorURL(connectBase, "twitter", "", "", "Invalid or expired connection request."))
		return
	}

	if state.CodeVerifier == "" {
		log.Printf("twitter: missing code_verifier in state")
		c.Redirect(302, CallbackErrorURL(connectBase, "twitter", state.CallbackScheme, state.App, "PKCE verification failed."))
		return
	}

	redirectURI := fmt.Sprintf("%s/connect/twitter/callback", constants.GetTrendlyBE())

	tokens, err := twitter.ExchangeCode(code, redirectURI, state.CodeVerifier)
	if err != nil {
		log.Printf("twitter: code exchange failed: %v", err)
		c.Redirect(302, CallbackErrorURL(connectBase, "twitter", state.CallbackScheme, state.App, "Failed to exchange authorization code."))
		return
	}

	user, err := twitter.GetMe(tokens.AccessToken)
	if err != nil {
		log.Printf("twitter: profile fetch failed: %v", err)
		c.Redirect(302, CallbackErrorURL(connectBase, "twitter", state.CallbackScheme, state.App, "Failed to fetch Twitter profile."))
		return
	}

	socialID := trendlymodels.SocialAccountID(trendlymodels.PlatformTwitter, user.Username)
	now := time.Now().Unix()

	social := &trendlymodels.SocialAccount{
		ID:              socialID,
		Platform:        trendlymodels.PlatformTwitter,
		UserID:          state.UserID,
		Username:        user.Username,
		DisplayName:     user.Name,
		ProfileImageURL: strings.Replace(user.ProfileImageURL, "_normal", "", 1),
		Bio:             user.Description,
		ProfileURL:      "https://twitter.com/" + user.Username,
		FollowerCount:   user.PublicMetrics.FollowersCount,
		FollowingCount:  user.PublicMetrics.FollowingCount,
		MediaCount:      user.PublicMetrics.TweetCount,
		ConnectedAt:     now,
		UpdatedAt:       now,
		RawProfile: map[string]interface{}{
			"id":           user.ID,
			"verified":     user.Verified,
			"verifiedType": user.VerifiedType,
			"listedCount":  user.PublicMetrics.ListedCount,
		},
	}

	socialToken := &trendlymodels.SocialToken{
		Platform:     trendlymodels.PlatformTwitter,
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
		log.Printf("twitter: firestore save failed: %v", saveErr)
		c.Redirect(302, CallbackErrorURL(connectBase, "twitter", state.CallbackScheme, state.App, "Failed to save connection. Please try again."))
		return
	}

	c.Redirect(302, CallbackSuccessURL(connectBase, "twitter", state.CallbackScheme, state.App))
}

// ── PKCE helpers ──────────────────────────────────────────────────────────────

// generatePKCE creates a random code_verifier and its S256 code_challenge.
func generatePKCE() (verifier, challenge string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)

	h := sha256.New()
	h.Write([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(h.Sum(nil))
	return
}
