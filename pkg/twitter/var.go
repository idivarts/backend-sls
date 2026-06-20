package twitter

import "os"

const (
	// Twitter / X OAuth 2.0 PKCE endpoints
	AuthURL  = "https://twitter.com/i/oauth2/authorize"
	TokenURL = "https://api.twitter.com/2/oauth2/token"
	RevokeURL = "https://api.twitter.com/2/oauth2/revoke"

	// Twitter API v2 base
	APIURL = "https://api.twitter.com/2"

	// OAuth 2.0 scopes
	ScopeTweetRead  = "tweet.read"
	ScopeUsersRead  = "users.read"
	ScopeOfflineAccess = "offline.access" // required for refresh tokens
)

var (
	ClientID     string
	ClientSecret string
)

func init() {
	ClientID = os.Getenv("TWITTER_CLIENT_ID")
	ClientSecret = os.Getenv("TWITTER_CLIENT_SECRET")
}
