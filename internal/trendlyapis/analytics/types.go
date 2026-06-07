// Package analytics serves unified, lightweight social-media reporting for
// brands. Phase 1 covers Meta (Instagram + Facebook) at full depth; other
// platforms surface follower counts only (Supported=false) until their
// analytics APIs are wired in a later phase.
//
// Insights are fetched on the fly from the Meta Graph API and memoised in a
// short-TTL Firestore cache (see cache.go). A separate daily cron snapshots
// top-line scalars for trend graphs (see snapshot.go).
package analytics

// Range is a supported analytics window.
type Range string

const (
	Range7d  Range = "7d"
	Range28d Range = "28d"
	Range90d Range = "90d"
)

// Unified metric bucket keys. Each platform fills whichever buckets it can.
const (
	BucketReach       = "reach"
	BucketImpressions = "impressions"
	BucketEngagement  = "engagement"
	BucketViews       = "views"
)

// MetricPoint is one point in a metric time series.
type MetricPoint struct {
	Date  string `json:"date"` // YYYY-MM-DD
	Value int64  `json:"value"`
}

// Metric is a single normalized metric (a unified bucket) for one account.
type Metric struct {
	Key       string        `json:"key"`
	Label     string        `json:"label"`
	Total     int64         `json:"total"`
	Series    []MetricPoint `json:"series,omitempty"`
	Available bool          `json:"available"`
}

// DemographicEntry is one slice of an audience breakdown (e.g. "18-24" → 1200).
type DemographicEntry struct {
	Label string `json:"label"`
	Value int64  `json:"value"`
}

// DemographicBucket groups audience entries by a single dimension.
type DemographicBucket struct {
	Dimension string             `json:"dimension"` // age, gender, country, city
	Entries   []DemographicEntry `json:"entries"`
}

// TopMedia is a single high-performing piece of content.
type TopMedia struct {
	ID           string `json:"id"`
	Caption      string `json:"caption,omitempty"`
	MediaType    string `json:"mediaType,omitempty"`
	MediaURL     string `json:"mediaUrl,omitempty"`
	ThumbnailURL string `json:"thumbnailUrl,omitempty"`
	Permalink    string `json:"permalink,omitempty"`
	Timestamp    int64  `json:"timestamp,omitempty"`
	Likes        int64  `json:"likes"`
	Comments     int64  `json:"comments"`
	Engagement   int64  `json:"engagement"`
}

// AccountAnalytics is the per-account analytics payload.
type AccountAnalytics struct {
	SocialID        string              `json:"socialId"`
	Platform        string              `json:"platform"`
	Username        string              `json:"username"`
	DisplayName     string              `json:"displayName"`
	ProfileImageURL string              `json:"profileImageUrl"`
	FollowerCount   int64               `json:"followerCount"`
	Metrics         map[string]Metric   `json:"metrics"`
	TopMedia        []TopMedia          `json:"topMedia"`
	Demographics    []DemographicBucket `json:"demographics,omitempty"`
	Range           string              `json:"range"`
	FetchedAt       int64               `json:"fetchedAt"`

	// Supported is false for platforms whose analytics aren't wired yet
	// (YouTube/LinkedIn/X in phase 1) — the client greys out detail for them.
	Supported bool `json:"supported"`
	// Stale indicates the payload was served from cache after a live refresh failed.
	Stale bool `json:"stale,omitempty"`
	// Error carries a per-account failure without failing the whole dashboard.
	Error string `json:"error,omitempty"`
}

// Overview is the unified, brand-wide analytics response.
type Overview struct {
	BrandID     string             `json:"brandId"`
	Range       string             `json:"range"`
	GeneratedAt int64              `json:"generatedAt"`
	Totals      map[string]int64   `json:"totals"` // followers, reach, impressions, engagement, views
	Accounts    []AccountAnalytics `json:"accounts"`
}
