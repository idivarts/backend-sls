package trendlyrdb

import (
	"errors"

	"github.com/idivarts/backend-sls/pkg/rdb"
	"github.com/lib/pq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	InstagramPostsTableName = "instagram_posts"
)

type InstagramPost struct {
	ID       string `gorm:"type:varchar(255)" db:"id" json:"id"`
	SocialID string `gorm:"type:varchar(255);not null;index" db:"social_id" json:"social_id"` // foreign key

	PostLocation   string  `gorm:"type:varchar(255)" db:"post_location" json:"post_location"`
	Type           string  `gorm:"type:varchar(50);index" db:"type" json:"type"`
	ShortCode      string  `gorm:"primaryKey;type:varchar(50);index" db:"short_code" json:"short_code"`
	Caption        string  `gorm:"type:text" db:"caption" json:"caption"`
	URL            string  `gorm:"type:text" db:"url" json:"url"`
	DisplayURL     string  `gorm:"type:text" db:"display_url" json:"display_url"`
	VideoURL       string  `gorm:"type:text" db:"video_url" json:"video_url"`
	LikesCount     int64   `gorm:"default:0;index" db:"likes_count" json:"likes_count"`
	CommentsCount  int64   `gorm:"default:0" db:"comments_count" json:"comments_count"`
	VideoViewCount int64   `gorm:"default:0" db:"video_view_count" json:"video_view_count"`
	VideoPlayCount int64   `gorm:"default:0" db:"video_play_count" json:"video_play_count"`
	VideoDuration  float64 `gorm:"type:double precision" db:"video_duration" json:"video_duration"`
	Timestamp      string  `gorm:"type:varchar(50);index" db:"timestamp" json:"timestamp"`
	LocationName   string  `gorm:"type:varchar(255)" db:"location_name" json:"location_name"`
	LocationID     string  `gorm:"type:varchar(100);index" db:"location_id" json:"location_id"`
	IsPinned       bool    `gorm:"default:false" db:"is_pinned" json:"is_pinned"`

	// Enhanced fields
	Alt                string          `gorm:"type:text" db:"alt" json:"alt"`
	Images             pq.StringArray  `gorm:"type:text[]" db:"images" json:"images"`
	IsCommentsDisabled bool            `gorm:"default:false" db:"is_comments_disabled" json:"is_comments_disabled"`
	AudioURL           string          `gorm:"type:text" db:"audio_url" json:"audio_url"`
	MusicInfo          *MusicInfo      `gorm:"type:jsonb;serializer:json" db:"music_info" json:"music_info"`
	Hashtags           pq.StringArray  `gorm:"type:text[]" db:"hashtags" json:"hashtags"`
	Mentions           pq.StringArray  `gorm:"type:text[]" db:"mentions" json:"mentions"`
	TaggedUsers        []User          `gorm:"type:jsonb;serializer:json" db:"tagged_users" json:"tagged_users"`
	FirstComment       string          `gorm:"type:text" db:"first_comment" json:"first_comment"`
	LatestComments     []Comment       `gorm:"type:jsonb;serializer:json" db:"latest_comments" json:"latest_comments"`
	ChildPosts         []InstagramPost `gorm:"type:jsonb;serializer:json" db:"child_posts" json:"child_posts,omitempty"`
}

type Comment struct {
	ID                 string  `json:"id"`
	Text               string  `json:"text"`
	OwnerUsername      string  `json:"owner_username"`
	OwnerProfilePicURL string  `json:"owner_profile_pic_url"`
	Timestamp          string  `json:"timestamp"`
	LikesCount         float64 `json:"likes_count"`
}

type MusicInfo struct {
	ArtistName        string `json:"artist_name"`
	SongName          string `json:"song_name"`
	UsesOriginalAudio bool   `json:"uses_original_audio"`
	AudioID           string `json:"audio_id"`
}

type User struct {
	FullName      string `json:"full_name"`
	ID            string `json:"id"`
	IsPrivate     bool   `json:"is_private"`
	IsVerified    bool   `json:"is_verified"`
	ProfilePicURL string `json:"profile_pic_url"`
	Username      string `json:"username"`
}

// TableName specifies the table name for GORM
func (InstagramPost) TableName() string {
	return InstagramPostsTableName
}

// Insert creates or updates an Instagram post
func (data *InstagramPost) Insert() error {
	return rdb.GormDB.Save(data).Error
}

// InsertMultiple bulk inserts or updates multiple Instagram posts
func (_ InstagramPost) InsertMultiple(posts []InstagramPost) error {
	return rdb.GormDB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(posts, 100).Error
}

// Get retrieves a single Instagram post by ID
func (data *InstagramPost) Get(id string) error {
	err := rdb.GormDB.Where("id = ?", id).First(data).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("post not found")
		}
		return err
	}
	return nil
}

// GetBySocialID retrieves all posts for a specific social profile
func (_ InstagramPost) GetBySocialID(socialID string, limit int) ([]InstagramPost, error) {
	var posts []InstagramPost
	query := rdb.GormDB.Where("social_id = ?", socialID).Order("timestamp DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&posts).Error
	return posts, err
}

// GetByShortCode retrieves a post by Instagram shortcode
func (data *InstagramPost) GetByShortCode(shortCode string) error {
	err := rdb.GormDB.Where("short_code = ?", shortCode).First(data).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("post not found")
		}
		return err
	}
	return nil
}

// GetPaginated retrieves paginated Instagram posts
func (_ InstagramPost) GetPaginated(offset, limit int) ([]InstagramPost, error) {
	var posts []InstagramPost
	err := rdb.GormDB.
		Order("timestamp DESC").
		Limit(limit).
		Offset(offset).
		Find(&posts).Error

	return posts, err
}

// GetMultiple retrieves multiple Instagram posts by IDs
func (_ InstagramPost) GetMultiple(ids []string) ([]InstagramPost, error) {
	var posts []InstagramPost
	err := rdb.GormDB.Where("id IN ?", ids).Find(&posts).Error
	return posts, err
}

// Update updates specific fields of an Instagram post
func (data *InstagramPost) Update(updates map[string]interface{}) error {
	return rdb.GormDB.Model(data).Updates(updates).Error
}

// Delete deletes an Instagram post
func (data *InstagramPost) Delete() error {
	return rdb.GormDB.Delete(data).Error
}

// Count returns the total number of Instagram posts
func (InstagramPost) Count() (int64, error) {
	var count int64
	err := rdb.GormDB.Model(&InstagramPost{}).Count(&count).Error
	return count, err
}

// CountBySocialID returns the number of posts for a specific social profile
func (InstagramPost) CountBySocialID(socialID string) (int64, error) {
	var count int64
	err := rdb.GormDB.Model(&InstagramPost{}).Where("social_id = ?", socialID).Count(&count).Error
	return count, err
}
