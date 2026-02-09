package trendlyrdb

import (
	"errors"

	"github.com/google/uuid"
	"github.com/idivarts/backend-sls/pkg/rdb"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

const (
	SocialsTableName = "socials"
)

type Socials struct {
	ID string `gorm:"primaryKey;type:varchar(36)" db:"id" json:"id"`

	Username     string `gorm:"type:varchar(255);not null" db:"username" json:"username"`
	Name         string `gorm:"type:varchar(255)" db:"name" json:"name"`
	Bio          string `gorm:"type:text" db:"bio" json:"bio"`
	ProfilePic   string `gorm:"type:text" db:"profile_pic" json:"profile_pic"`
	ProfilePicHD string `gorm:"type:text" db:"profile_pic_hd" json:"profile_pic_hd"`
	Category     string `gorm:"type:varchar(100)" db:"category" json:"category"`

	SocialType      string `gorm:"type:varchar(50);not null" db:"social_type" json:"social_type"`
	ProfileVerified bool   `gorm:"default:false" db:"profile_verified" json:"profile_verified"`

	FollowerCount  int64 `gorm:"default:0" db:"follower_count" json:"follower_count"`
	FollowingCount int64 `gorm:"default:0" db:"following_count" json:"following_count"`
	ContentCount   int64 `gorm:"default:0" db:"content_count" json:"content_count"`

	// Analytics/Metrics
	ViewsCount      int64   `gorm:"default:0" db:"views_count" json:"views_count"`
	EngagementCount int64   `gorm:"default:0" db:"engagement_count" json:"engagement_count"`
	EngagementRate  float32 `gorm:"type:real;default:0.0" db:"engagement_rate" json:"engagement_rate"`
	AverageViews    float32 `gorm:"type:real;default:0.0" db:"average_views" json:"average_views"`
	AverageLikes    float32 `gorm:"type:real;default:0.0" db:"average_likes" json:"average_likes"`
	AverageComments float32 `gorm:"type:real;default:0.0" db:"average_comments" json:"average_comments"`

	// JSONB field for links - array of Link objects
	Links []Links `gorm:"type:jsonb;serializer:json" db:"links" json:"links"`

	// AI-Deduced Fields
	Gender   string         `gorm:"type:varchar(50)" db:"gender" json:"gender"`
	Location string         `gorm:"type:varchar(255)" db:"location" json:"location"`
	Niches   pq.StringArray `gorm:"type:text[]" db:"niches" json:"niches"`

	QualityScore int `gorm:"type:integer" db:"quality_score" json:"quality_score"`

	// Metadata
	AddedBy        string `gorm:"type:varchar(255)" db:"added_by" json:"added_by"`
	CreationTime   int64  `gorm:"type:bigint" db:"creation_time" json:"creation_time"`
	LastUpdateTime int64  `gorm:"type:bigint" db:"last_update_time" json:"last_update_time"`

	// Enhanced Profile
	ExternalId string `gorm:"type:varchar(255)" db:"external_id" json:"external_id"`
}

type Links struct {
	Title    string `db:"title" json:"title"`
	URL      string `db:"url" json:"url"`
	LinkType string `db:"link_type" json:"link_type"`
}

// TableName specifies the table name for GORM
func (Socials) TableName() string {
	return SocialsTableName
}

// GetID generates a deterministic UUID from social_type + username
func (data *Socials) GetID() string {
	ID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(data.SocialType+data.Username))
	return ID.String()
}

// Insert creates or updates a social profile
func (data *Socials) Insert() error {
	data.ID = data.GetID()

	// Use GORM's Clauses for upsert (ON CONFLICT DO UPDATE)
	return rdb.GormDB.Save(data).Error
}

// InsertMultiple bulk inserts or updates multiple social profiles
func (_ Socials) InsertMultiple(socials []Socials) error {
	// Generate IDs for all records
	for i := range socials {
		socials[i].ID = socials[i].GetID()
	}

	// Use GORM's CreateInBatches for efficient bulk insert
	return rdb.GormDB.CreateInBatches(socials, 100).Error
}

// GetPaginated retrieves paginated social profiles
func (_ Socials) GetPaginated(offset, limit int) ([]Socials, error) {
	var results []Socials
	err := rdb.GormDB.
		Order("last_update_time DESC").
		Limit(limit).
		Offset(offset).
		Find(&results).Error

	return results, err
}

// Get retrieves a single social profile by ID
func (data *Socials) Get(id string) error {
	err := rdb.GormDB.Where("id = ?", id).First(data).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("social not found")
		}
		return err
	}
	return nil
}

// GetMultiple retrieves multiple social profiles by IDs
func (_ Socials) GetMultiple(ids []string) ([]Socials, error) {
	var results []Socials
	err := rdb.GormDB.Where("id IN ?", ids).Find(&results).Error
	return results, err
}

// GetInstagram retrieves an Instagram profile by username
func (data *Socials) GetInstagram(username string) error {
	data.Username = username
	data.SocialType = "instagram"
	id := data.GetID()

	return data.Get(id)
}

// Update updates specific fields of a social profile
func (data *Socials) Update(updates map[string]interface{}) error {
	return rdb.GormDB.Model(data).Updates(updates).Error
}

// Delete soft deletes a social profile
func (data *Socials) Delete() error {
	return rdb.GormDB.Delete(data).Error
}

func (Socials) Count() (int64, error) {
	var count int64
	err := rdb.GormDB.Model(&Socials{}).Count(&count).Error
	return count, err
}
