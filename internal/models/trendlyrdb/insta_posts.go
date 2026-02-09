package trendlyrdb

const (
	InstagramPostsTableName = "`instagram_posts`"
)

type Comment struct {
	ID                 string  `db:"id" bigquery:"id" json:"id" firestore:"id"`
	Text               string  `db:"text" bigquery:"text" json:"text" firestore:"text"`
	OwnerUsername      string  `db:"owner_username" bigquery:"owner_username" json:"owner_username" firestore:"owner_username"`
	OwnerProfilePicURL string  `db:"owner_profile_pic_url" bigquery:"owner_profile_pic_url" json:"owner_profile_pic_url" firestore:"owner_profile_pic_url"`
	Timestamp          string  `db:"timestamp" bigquery:"timestamp" json:"timestamp" firestore:"timestamp"`
	LikesCount         float64 `db:"likes_count" bigquery:"likes_count" json:"likes_count" firestore:"likes_count"`
}

type InstagramPost struct {
	ID       string `db:"id" bigquery:"id" json:"id" firestore:"id"`
	SocialID string `db:"social_id" bigquery:"social_id" json:"social_id" firestore:"social_id"` //foreign key

	PostLocation   string  `db:"post_location" bigquery:"post_location" json:"post_location" firestore:"post_location"`
	Type           string  `db:"type" bigquery:"type" json:"type" firestore:"type"`
	ShortCode      string  `db:"short_code" bigquery:"short_code" json:"short_code" firestore:"short_code"`
	Caption        string  `db:"caption" bigquery:"caption" json:"caption" firestore:"caption"`
	URL            string  `db:"url" bigquery:"url" json:"url" firestore:"url"`
	DisplayURL     string  `db:"display_url" bigquery:"display_url" json:"display_url" firestore:"display_url"`
	VideoURL       string  `db:"video_url" bigquery:"video_url" json:"video_url" firestore:"video_url"`
	LikesCount     int64   `db:"likes_count" bigquery:"likes_count" json:"likes_count" firestore:"likes_count"`
	CommentsCount  int64   `db:"comments_count" bigquery:"comments_count" json:"comments_count" firestore:"comments_count"`
	VideoViewCount int64   `db:"video_view_count" bigquery:"video_view_count" json:"video_view_count" firestore:"video_view_count"`
	VideoPlayCount int64   `db:"video_play_count" bigquery:"video_play_count" json:"video_play_count" firestore:"video_play_count"`
	VideoDuration  float64 `db:"video_duration" bigquery:"video_duration" json:"video_duration" firestore:"video_duration"`
	Timestamp      string  `db:"timestamp" bigquery:"timestamp" json:"timestamp" firestore:"timestamp"`
	LocationName   string  `db:"location_name" bigquery:"location_name" json:"location_name" firestore:"location_name"`
	LocationID     string  `db:"location_id" bigquery:"location_id" json:"location_id" firestore:"location_id"`
	IsPinned       bool    `db:"is_pinned" bigquery:"is_pinned" json:"is_pinned" firestore:"is_pinned"`

	// Enhanced fields
	Alt                string          `db:"alt" bigquery:"alt" json:"alt" firestore:"alt"`
	Images             []string        `db:"images" bigquery:"images" json:"images" firestore:"images"`
	IsCommentsDisabled bool            `db:"is_comments_disabled" bigquery:"is_comments_disabled" json:"is_comments_disabled" firestore:"is_comments_disabled"`
	AudioURL           string          `db:"audio_url" bigquery:"audio_url" json:"audio_url" firestore:"audio_url"`
	MusicInfo          MusicInfo       `db:"music_info" bigquery:"music_info" json:"music_info" firestore:"music_info"`
	Hashtags           []string        `db:"hashtags" bigquery:"hashtags" json:"hashtags" firestore:"hashtags"`
	Mentions           []string        `db:"mentions" bigquery:"mentions" json:"mentions" firestore:"mentions"`
	TaggedUsers        []User          `db:"tagged_users" bigquery:"tagged_users" json:"tagged_users" firestore:"tagged_users"`
	FirstComment       string          `db:"first_comment" bigquery:"first_comment" json:"first_comment" firestore:"first_comment"`
	LatestComments     []Comment       `db:"latest_comments" bigquery:"latest_comments" json:"latest_comments" firestore:"latest_comments"`
	ChildPosts         []InstagramPost `db:"child_posts,omitempty" bigquery:"child_posts,omitempty" json:"child_posts,omitempty" firestore:"child_posts,omitempty"`
}
