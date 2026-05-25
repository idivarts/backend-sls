package trendlymodels

// ─── Platform constants ────────────────────────────────────────────────────────

// Platform identifies a social media platform using a stable string key.
// Using string rather than int avoids remapping issues as platforms are added.
type Platform = string

const (
	PlatformInstagram Platform = "instagram"
	PlatformFacebook  Platform = "facebook"
	PlatformYouTube   Platform = "youtube"
	PlatformLinkedIn  Platform = "linkedin"
	PlatformTwitter   Platform = "twitter"
)

// ─── SocialV2 (users/{userId}/socialsV2/{id}) ─────────────────────────────────

// SocialV2 is the forward-only public social account document.
// It lives alongside the legacy `socials` sub-collection; the old collection
// is never written to for new connections.
type SocialV2 struct {
	// Core identity
	ID       string   `json:"id" firestore:"id"`             // deterministic: platform:username
	Platform Platform `json:"platform" firestore:"platform"` // "instagram", "facebook", etc.
	UserID   string   `json:"userId" firestore:"userId"`

	// Profile info (refreshed on each reconnect / token refresh)
	Username        string `json:"username" firestore:"username"`
	DisplayName     string `json:"displayName" firestore:"displayName"`
	ProfileImageURL string `json:"profileImageURL" firestore:"profileImageURL"`
	Bio             string `json:"bio,omitempty" firestore:"bio"`
	ProfileURL      string `json:"profileURL,omitempty" firestore:"profileURL"` // e.g. https://instagram.com/{handle}

	// Metrics (updated by social sync job)
	FollowerCount  int64 `json:"followerCount" firestore:"followerCount"`
	FollowingCount int64 `json:"followingCount" firestore:"followingCount"`
	MediaCount     int64 `json:"mediaCount" firestore:"mediaCount"` // posts/videos

	// Connection metadata
	ConnectedAt int64 `json:"connectedAt" firestore:"connectedAt"` // Unix timestamp
	UpdatedAt   int64 `json:"updatedAt" firestore:"updatedAt"`     // last profile sync

	// Platform-specific raw profile blob (optional, for future use)
	// Stored as map so we don't need to import platform packages here.
	RawProfile map[string]interface{} `json:"rawProfile,omitempty" firestore:"rawProfile"`
}

// SocialV2ID returns the canonical document ID for a social account.
// Format: "{platform}:{username}" — deterministic and collision-free.
func SocialV2ID(platform Platform, username string) string {
	return platform + ":" + username
}

// ─── SocialV2Private (users/{userId}/socialsV2Private/{id}) ───────────────────

// SocialV2Private holds sensitive token data in a separate sub-collection.
// Access rules should restrict reads to server-side only (Firebase security rules).
type SocialV2Private struct {
	Platform     Platform `json:"platform" firestore:"platform"`
	AccessToken  string   `json:"accessToken" firestore:"accessToken"`
	RefreshToken string   `json:"refreshToken,omitempty" firestore:"refreshToken"` // empty for non-refreshable tokens
	TokenExpiry  int64    `json:"tokenExpiry" firestore:"tokenExpiry"`             // Unix timestamp; 0 = no expiry
	Scopes       []string `json:"scopes,omitempty" firestore:"scopes"`
	// PKCE verifier is only stored transiently during the OAuth flow (not persisted here).
}
