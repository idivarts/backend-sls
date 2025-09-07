package trendlybq

type Socials struct {
	ID         string `db:"id" bigquery:"id"`
	SocialType string `db:"social_type" bigquery:"social_type"`

	Gender     string   `db:"gender" bigquery:"gender"`
	Categories []string `db:"categories" bigquery:"categories"`
	Location   string   `db:"location" bigquery:"location"`

	FollowerCount  int `db:"follower_count" bigquery:"follower_count"`
	ContentCount   int `db:"follower_count" bigquery:"follower_count"`
	ViewsCount     int `db:"views_count" bigquery:"views_count"`            //views
	EnagamentCount int `db:"engagement_count" bigquery:"engagements_count"` //engagement

	AverageViews    float32 `db:"average_views" bigquery:"average_views"`
	AverageLikes    float32 `db:"average_likes" bigquery:"average_likes"`
	AverageComments float32 `db:"average_comments" bigquery:"average_comments"`
	QualityScore    int     `db:"quality_score" bigquery:"quality_score"`
	EngagementRate  float32 `db:"engagement_rate" bigquery:"engagement_rate"`

	Name string `db:"name" bigquery:"name"`
	Bio  string `db:"bio" bigquery:"bio"`

	ProfileVerified bool `db:"profile_verified" bigquery:"profile_verified"`
	HasContacts     bool `db:"has_contacts" bigquery:"has_contacts"`

	Reels []Reel `db:"reels" bigquery:"reels"`

	AddedBy string `db:"added_by" bigquery:"added_by"`

	CreationTime   int64 `db:"creation_time" bigquery:"creation_time"`
	LastUpdateTime int64 `db:"last_update_time" bigquery:"last_update_time"`
}

type Reel struct {
	ThumbnailURL  string `db:"thumbnail_url" bigquery:"thumbnail_url"`
	Caption       string `db:"caption" bigquery:"caption"`
	URL           string `db:"url" bigquery:"url"`
	Pinned        bool   `db:"pinned" bigquery:"pinned"`
	ViewsCount    *int   `db:"views_count" bigquery:"views_count"`
	LikesCount    *int   `db:"likes_count" bigquery:"likes_count"`
	CommentsCount *int   `db:"comments_count" bigquery:"comments_count"`
}
