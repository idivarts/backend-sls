package youtube

import "os"

const (
	// Google OAuth2 endpoints
	AuthURL  = "https://accounts.google.com/o/oauth2/v2/auth"
	TokenURL = "https://oauth2.googleapis.com/token"
	RevokeURL = "https://oauth2.googleapis.com/revoke"

	// YouTube Data API v3
	APIURL     = "https://www.googleapis.com/youtube/v3"
	AnalyticsURL = "https://youtubeanalytics.googleapis.com/v2"
	// UploadURL is the resumable-upload host for videos.insert (distinct from APIURL).
	UploadURL = "https://www.googleapis.com/upload/youtube/v3/videos"

	// OAuth scopes
	ScopeYouTubeReadonly  = "https://www.googleapis.com/auth/youtube.readonly"
	ScopeYTAnalytics      = "https://www.googleapis.com/auth/yt-analytics.readonly"
	ScopeUserInfoProfile  = "https://www.googleapis.com/auth/userinfo.profile"
	ScopeUserInfoEmail    = "https://www.googleapis.com/auth/userinfo.email"
	// ScopeYouTubeUpload is required for videos.insert (posting). ScopeYouTubeForceSSL
	// covers thumbnails.set + comment moderation. Both are sensitive scopes →
	// require Google OAuth verification (see docs/social-expansion-dashboard-setup.md §2).
	ScopeYouTubeUpload   = "https://www.googleapis.com/auth/youtube.upload"
	ScopeYouTubeForceSSL = "https://www.googleapis.com/auth/youtube.force-ssl"
)

var (
	ClientID     string
	ClientSecret string
)

func init() {
	ClientID = os.Getenv("YOUTUBE_CLIENT_ID")
	ClientSecret = os.Getenv("YOUTUBE_CLIENT_SECRET")
}
