package trendlyrdb

const (
	InstagramPostsTableName = "`instagram_posts`"
)

type InstagramPost struct {
	ID       string `db:"id" json:"id"`
	SocialID string `db:"social_id" json:"social_id"` //foreign key

	PostLocation   string  `db:"post_location" json:"post_location"`
	Type           string  `db:"type" json:"type"`
	ShortCode      string  `db:"short_code" json:"short_code"`
	Caption        string  `db:"caption" json:"caption"`
	URL            string  `db:"url" json:"url"`
	DisplayURL     string  `db:"display_url" json:"display_url"`
	VideoURL       string  `db:"video_url" json:"video_url"`
	LikesCount     int64   `db:"likes_count" json:"likes_count"`
	CommentsCount  int64   `db:"comments_count" json:"comments_count"`
	VideoViewCount int64   `db:"video_view_count" json:"video_view_count"`
	VideoPlayCount int64   `db:"video_play_count" json:"video_play_count"`
	VideoDuration  float64 `db:"video_duration" json:"video_duration"`
	Timestamp      string  `db:"timestamp" json:"timestamp"`
	LocationName   string  `db:"location_name" json:"location_name"`
	LocationID     string  `db:"location_id" json:"location_id"`
	IsPinned       bool    `db:"is_pinned" json:"is_pinned"`

	// Enhanced fields
	Alt                string          `db:"alt" json:"alt"`
	Images             []string        `db:"images" json:"images"`
	IsCommentsDisabled bool            `db:"is_comments_disabled" json:"is_comments_disabled"`
	AudioURL           string          `db:"audio_url" json:"audio_url"`
	MusicInfo          MusicInfo       `db:"music_info" json:"music_info"`
	Hashtags           []string        `db:"hashtags" json:"hashtags"`
	Mentions           []string        `db:"mentions" json:"mentions"`
	TaggedUsers        []User          `db:"tagged_users" json:"tagged_users"`
	FirstComment       string          `db:"first_comment" json:"first_comment"`
	LatestComments     []Comment       `db:"latest_comments" json:"latest_comments"`
	ChildPosts         []InstagramPost `db:"child_posts,omitempty" json:"child_posts,omitempty"`
}

type Comment struct {
	ID                 string  `db:"id" json:"id"`
	Text               string  `db:"text" json:"text"`
	OwnerUsername      string  `db:"owner_username" json:"owner_username"`
	OwnerProfilePicURL string  `db:"owner_profile_pic_url" json:"owner_profile_pic_url"`
	Timestamp          string  `db:"timestamp" json:"timestamp"`
	LikesCount         float64 `db:"likes_count" json:"likes_count"`
}
type MusicInfo struct {
	ArtistName        string `db:"artist_name" json:"artist_name"`
	SongName          string `db:"song_name" json:"song_name"`
	UsesOriginalAudio bool   `db:"uses_original_audio" json:"uses_original_audio"`
	AudioID           string `db:"audio_id" json:"audio_id"`
}
type User struct {
	FullName      string `db:"full_name" json:"full_name"`
	ID            string `db:"id" json:"id"`
	IsPrivate     bool   `db:"is_private" json:"is_private"`
	IsVerified    bool   `db:"is_verified" json:"is_verified"`
	ProfilePicURL string `db:"profile_pic_url" json:"profile_pic_url"`
	Username      string `db:"username" json:"username"`
}
