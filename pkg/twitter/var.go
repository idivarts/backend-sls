package twitter

import "os"

const (
	// Twitter / X OAuth 2.0 PKCE endpoints
	AuthURL   = "https://x.com/i/oauth2/authorize"
	TokenURL  = "https://api.twitter.com/2/oauth2/token"
	RevokeURL = "https://api.twitter.com/2/oauth2/revoke"

	// Twitter API v2 base
	APIURL = "https://api.twitter.com/2"

	// OAuth 2.0 scopes
	ScopeTweetRead     = "tweet.read"
	ScopeUsersRead     = "users.read"
	ScopeOfflineAccess = "offline.access" // required for refresh tokens
	ScopeTweetWrite    = "tweet.write"    // create tweets / replies (posting + media inbox)
	ScopeMediaWrite    = "media.write"    // v2 chunked media upload (posting)
	ScopeDMRead        = "dm.read"        // read DM events (messaging inbox)
	ScopeDMWrite       = "dm.write"       // send DMs (messaging inbox)
)

var (
	ClientID     string
	ClientSecret string
)

func init() {
	ClientID = os.Getenv("TWITTER_CLIENT_ID")
	ClientSecret = os.Getenv("TWITTER_CLIENT_SECRET")
}
