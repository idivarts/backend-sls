package linkedin

import "os"

const (
	// LinkedIn OAuth 2.0 endpoints
	AuthURL   = "https://www.linkedin.com/oauth/v2/authorization"
	TokenURL  = "https://www.linkedin.com/oauth/v2/accessToken"
	RevokeURL = "https://www.linkedin.com/oauth/v2/revoke"

	// LinkedIn API base
	APIURL = "https://api.linkedin.com/v2"

	// OAuth scopes (OpenID Connect + profile read)
	ScopeOpenID  = "openid"
	ScopeProfile = "profile"
	ScopeEmail   = "email"
	// r_basicprofile is the legacy scope; w_member_social for posting
	ScopeBasicProfile = "r_basicprofile"
)

var (
	ClientID     string
	ClientSecret string
)

func init() {
	ClientID = os.Getenv("LINKEDIN_CLIENT_ID")
	ClientSecret = os.Getenv("LINKEDIN_CLIENT_SECRET")
}
