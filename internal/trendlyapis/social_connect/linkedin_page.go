package social_connect

import (
	"crypto/rand"
	"encoding/hex"
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

// linkedinPageScopesRequired are the Community Management API scopes requested on
// the dedicated CMA app for Company/Showcase Page connect. The member must be a
// page ADMINISTRATOR. See docs/linkedin-pages-cma-setup.md.
var linkedinPageScopesRequired = strings.Join([]string{
	linkedin.ScopeBasicProfile,
	linkedin.ScopeOrgAdmin,
	linkedin.ScopeOrgSocial,
	linkedin.ScopeOrgSocialW,
	linkedin.ScopeOrgFollowers,
}, " ")

// LinkedInPageInit redirects to the CMA app's OAuth consent for Company Pages.
func LinkedInPageInit(c *gin.Context) {
	userId, _ := middlewares.GetUserId(c)
	callbackScheme := c.Query("callbackScheme")
	app := c.Query("app")
	brandId := c.Query("brandId")
	if callbackScheme == "" || app == "" {
		c.JSON(400, gin.H{"error": "callbackScheme and app are required"})
		return
	}
	if brandId == "" {
		c.JSON(400, gin.H{"error": "brandId is required to connect a LinkedIn Page"})
		return
	}

	state := &OAuthState{
		UserID:         userId,
		Platform:       trendlymodels.PlatformLinkedInPage,
		App:            app,
		CallbackScheme: callbackScheme,
		BrandID:        brandId,
	}
	encodedState, err := state.Encode()
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to encode state"})
		return
	}

	redirectURI := fmt.Sprintf("%s/connect/linkedin_page/callback", constants.GetTrendlyBE())
	authURL := fmt.Sprintf(
		"%s?response_type=code&client_id=%s&redirect_uri=%s&scope=%s&state=%s",
		linkedin.AuthURL,
		url.QueryEscape(linkedin.CMClientID),
		url.QueryEscape(redirectURI),
		url.QueryEscape(linkedinPageScopesRequired),
		url.QueryEscape(encodedState),
	)
	c.Redirect(302, authURL)
}

// LinkedInPageCallback handles the CMA-app OAuth callback: it mints the member
// token, lists the Pages the member administers, stashes a short-lived session,
// and redirects to the connect-portal page-picker.
// GET /connect/linkedin_page/callback?code=...&state=...
func LinkedInPageCallback(c *gin.Context) {
	connectBase := constants.GetConnectFronted()

	if errParam := c.Query("error"); errParam != "" {
		log.Printf("linkedin_page: OAuth error: %s - %s", errParam, c.Query("error_description"))
		c.Redirect(302, CallbackErrorURL(connectBase, "linkedin_page", "", "", c.Query("error_description")))
		return
	}

	state, err := DecodeState(c.Query("state"))
	if err != nil {
		log.Printf("linkedin_page: invalid state: %v", err)
		c.Redirect(302, CallbackErrorURL(connectBase, "linkedin_page", "", "", "Invalid or expired connection request."))
		return
	}

	redirectURI := fmt.Sprintf("%s/connect/linkedin_page/callback", constants.GetTrendlyBE())
	tokens, err := linkedin.ExchangeCodeCM(c.Query("code"), redirectURI)
	if err != nil {
		log.Printf("linkedin_page: code exchange failed: %v", err)
		c.Redirect(302, CallbackErrorURL(connectBase, "linkedin_page", state.CallbackScheme, state.App, "Failed to exchange authorization code."))
		return
	}

	profile, err := linkedin.GetMe(tokens.AccessToken)
	if err != nil {
		log.Printf("linkedin_page: profile fetch failed: %v", err)
		c.Redirect(302, CallbackErrorURL(connectBase, "linkedin_page", state.CallbackScheme, state.App, "Failed to fetch LinkedIn profile."))
		return
	}
	parts := strings.Split(profile.Sub, ":")
	memberID := parts[len(parts)-1]

	orgs, err := linkedin.ListAdministeredOrgs(tokens.AccessToken)
	if err != nil {
		log.Printf("linkedin_page: list admin orgs failed: %v", err)
		c.Redirect(302, CallbackErrorURL(connectBase, "linkedin_page", state.CallbackScheme, state.App, "Couldn't read your LinkedIn Pages. Ensure the Community Management API is approved."))
		return
	}
	if len(orgs) == 0 {
		c.Redirect(302, CallbackErrorURL(connectBase, "linkedin_page", state.CallbackScheme, state.App, "You don't administer any LinkedIn Company Pages."))
		return
	}

	sessOrgs := make([]trendlymodels.LinkedInPageSessionOrg, 0, len(orgs))
	for _, o := range orgs {
		sessOrgs = append(sessOrgs, trendlymodels.LinkedInPageSessionOrg{
			URN:        o.URN,
			ID:         o.ID,
			Name:       o.Name,
			VanityName: o.VanityName,
			LogoURL:    o.LogoURL,
		})
	}

	sessionID, err := genSessionID()
	if err != nil {
		c.Redirect(302, CallbackErrorURL(connectBase, "linkedin_page", state.CallbackScheme, state.App, "Failed to start page selection."))
		return
	}
	sess := &trendlymodels.LinkedInPageSession{
		BrandID:        state.BrandID,
		App:            state.App,
		CallbackScheme: state.CallbackScheme,
		UserID:         state.UserID,
		MemberID:       memberID,
		AccessToken:    tokens.AccessToken,
		RefreshToken:   tokens.RefreshToken,
		TokenExpiry:    tokens.ExpiresAt(),
		Scopes:         strings.Split(tokens.Scope, " "),
		Orgs:           sessOrgs,
	}
	if err := trendlymodels.CreateLinkedInPageSession(sessionID, sess); err != nil {
		log.Printf("linkedin_page: session save failed: %v", err)
		c.Redirect(302, CallbackErrorURL(connectBase, "linkedin_page", state.CallbackScheme, state.App, "Failed to start page selection."))
		return
	}

	// Hand off to the connect portal's page-picker. `be` tells the (static) portal
	// exactly which backend issued the session, so its session/select calls hit
	// the right stage (prod vs dev) without hostname guessing.
	c.Redirect(302, fmt.Sprintf("%s/connect/select-pages?session=%s&be=%s",
		connectBase, url.QueryEscape(sessionID), url.QueryEscape(constants.GetTrendlyBE())))
}

// LinkedInPageSessionInfo returns the picker payload for a pending session (the
// admin-page list + return context). Public — guarded by the unguessable,
// short-lived session id. NO token is exposed.
// GET /connect/linkedin_page/session?session=...
func LinkedInPageSessionInfo(c *gin.Context) {
	id := c.Query("session")
	if id == "" {
		c.JSON(400, gin.H{"error": "session is required"})
		return
	}
	sess, err := trendlymodels.GetLinkedInPageSession(id)
	if err != nil {
		c.JSON(404, gin.H{"error": "session not found or expired"})
		return
	}
	c.JSON(200, gin.H{
		"orgs":           sess.Orgs,
		"app":            sess.App,
		"callbackScheme": sess.CallbackScheme,
	})
}

type linkedInPageSelectReq struct {
	Session string   `json:"session" binding:"required"`
	OrgIds  []string `json:"orgIds" binding:"required"`
}

// LinkedInPageSelect creates one page SocialAccount per chosen org, all sharing a
// single member token doc (tokenRef), then consumes the session. Public — guarded
// by the session id.
// POST /connect/linkedin_page/select  { session, orgIds[] }
func LinkedInPageSelect(c *gin.Context) {
	var req linkedInPageSelectReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	sess, err := trendlymodels.GetLinkedInPageSession(req.Session)
	if err != nil {
		c.JSON(404, gin.H{"error": "session not found or expired"})
		return
	}

	chosen := map[string]bool{}
	for _, id := range req.OrgIds {
		chosen[id] = true
	}

	now := time.Now().Unix()
	tokenDocID := "lipage_" + sess.MemberID
	sharedToken := &trendlymodels.SocialToken{
		Platform:     trendlymodels.PlatformLinkedInPage,
		AccessToken:  sess.AccessToken,
		RefreshToken: sess.RefreshToken,
		TokenExpiry:  sess.TokenExpiry,
		Scopes:       sess.Scopes,
	}

	accounts := make([]trendlymodels.SocialAccount, 0, len(req.OrgIds))
	for _, o := range sess.Orgs {
		if !chosen[o.ID] {
			continue
		}
		handle := firstNonEmptyStr(o.VanityName, o.ID)
		accounts = append(accounts, trendlymodels.SocialAccount{
			ID:                trendlymodels.SocialAccountID(trendlymodels.PlatformLinkedInPage, o.ID),
			Platform:          trendlymodels.PlatformLinkedInPage,
			UserID:            sess.UserID,
			PlatformAccountID: o.ID,
			Username:          handle,
			DisplayName:       o.Name,
			ProfileImageURL:   o.LogoURL,
			ProfileURL:        "https://www.linkedin.com/company/" + handle,
			AccountType:       "organization",
			VanityName:        o.VanityName,
			ConnectedAt:       now,
			UpdatedAt:         now,
			RawProfile: map[string]interface{}{
				"orgUrn":   o.URN,
				"memberId": sess.MemberID,
			},
		})
	}
	if len(accounts) == 0 {
		c.JSON(400, gin.H{"error": "no valid pages selected"})
		return
	}

	if err := trendlymodels.SaveBrandPageAccounts(sess.BrandID, accounts, sharedToken, tokenDocID); err != nil {
		log.Printf("linkedin_page: save pages failed: %v", err)
		c.JSON(500, gin.H{"error": "failed to save selected pages"})
		return
	}
	_ = trendlymodels.DeleteLinkedInPageSession(req.Session) // best-effort cleanup

	c.JSON(200, gin.H{
		"ok":             true,
		"count":          len(accounts),
		"app":            sess.App,
		"callbackScheme": sess.CallbackScheme,
	})
}

// genSessionID returns a random 32-byte hex id used as the page-session key.
func genSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
