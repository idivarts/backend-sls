package linkedin

import "os"

const (
	// LinkedIn OAuth 2.0 endpoints
	AuthURL   = "https://www.linkedin.com/oauth/v2/authorization"
	TokenURL  = "https://www.linkedin.com/oauth/v2/accessToken"
	RevokeURL = "https://www.linkedin.com/oauth/v2/revoke"

	// LinkedIn API base
	APIURL = "https://api.linkedin.com/v2"
	// RestBaseURL is the versioned REST base used by the Posts and Images APIs.
	// Calls against it require the LinkedIn-Version and X-Restli-Protocol-Version
	// headers (see pkg/linkedin/posts.go).
	RestBaseURL = "https://api.linkedin.com/rest"

	// OAuth scopes (OpenID Connect + profile read)
	ScopeOpenID  = "openid"
	ScopeProfile = "profile"
	ScopeEmail   = "email"
	// ScopeMemberSocial allows posting on behalf of the authenticated member
	// (personal profile). Granted by the self-serve "Share on LinkedIn" product.
	ScopeMemberSocial = "w_member_social"
	// r_basicprofile is the legacy scope
	ScopeBasicProfile = "r_basicprofile"

	// defaultAPIVersion is a supported LinkedIn-Version month (YYYYMM) used when
	// LINKEDIN_API_VERSION is not set. Keep in sync with the version enabled on
	// the LinkedIn developer app.
	//
	// ⚠️ LinkedIn sunsets each versioned month ~12 months after release, so this
	// must be bumped roughly yearly. "202506" was sunset by mid-2026; "202606" is
	// the current month and supports the Posts API multiImage (carousel) content.
	defaultAPIVersion = "202606"
)

var (
	ClientID     string
	ClientSecret string
	// APIVersion is the LinkedIn-Version header value (YYYYMM) sent on versioned
	// /rest calls. Overridable via LINKEDIN_API_VERSION.
	APIVersion string
)

func init() {
	ClientID = os.Getenv("LINKEDIN_CLIENT_ID")
	ClientSecret = os.Getenv("LINKEDIN_CLIENT_SECRET")
	APIVersion = os.Getenv("LINKEDIN_API_VERSION")
	if APIVersion == "" {
		APIVersion = defaultAPIVersion
	}
}
