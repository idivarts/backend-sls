package reddit

import "os"

const (
	// Reddit OAuth 2.0 endpoints. Authorization + token live on www.reddit.com;
	// all authenticated API calls go to oauth.reddit.com.
	AuthURL   = "https://www.reddit.com/api/v1/authorize"
	TokenURL  = "https://www.reddit.com/api/v1/access_token"
	RevokeURL = "https://www.reddit.com/api/v1/revoke_token"

	// APIURL is the OAuth API host (all calls require a Bearer token + User-Agent).
	APIURL = "https://oauth.reddit.com"

	// OAuth scopes (space-separated in the authorize URL).
	ScopeIdentity        = "identity"        // /api/v1/me
	ScopeSubmit          = "submit"          // submit posts + comments
	ScopeRead            = "read"            // read listings/comments
	ScopePrivateMessages = "privatemessages" // read/compose PMs (read-only since Aug 2025)
	ScopeEdit            = "edit"            // edit/delete own things
	ScopeHistory         = "history"         // read own submission/comment history
)

var (
	ClientID     string
	ClientSecret string
	// UserAgent is sent on EVERY request. Reddit requires a unique, descriptive
	// User-Agent and bans generic/spoofed ones. Format:
	//   web:<app-id>:<version> (by /u/<reddit-username>)
	UserAgent string
)

func init() {
	ClientID = os.Getenv("REDDIT_CLIENT_ID")
	ClientSecret = os.Getenv("REDDIT_CLIENT_SECRET")
	UserAgent = os.Getenv("REDDIT_USER_AGENT")
	if UserAgent == "" {
		UserAgent = "web:trendly-ai-social-planner:v1.0 (by /u/trendlyapp)"
	}
}
