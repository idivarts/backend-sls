package trendlyrdb

import (
	"errors"

	"github.com/idivarts/backend-sls/pkg/rdb"
	"github.com/lib/pq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	InfluencersTableName = "influencers"
)

var excludedInfluencerIDs = []string{
	"MvLmVKwUcXXZXfBfQHSnq5udnaO2",
	"mmUwj1YlPUVn0h2hlN4qVw1bEZo1",
	"jEZf51INayY4ZcJs2ck0XWR8Ptj2",
}

type Influencers struct {
	ID string `gorm:"primaryKey;type:varchar(36)" db:"id" json:"id"`

	Location string `gorm:"type:varchar(255)" db:"location" json:"location"`

	Categories               pq.StringArray `gorm:"type:text[]" db:"categories" json:"categories"`
	Languages                pq.StringArray `gorm:"type:text[]" db:"languages" json:"languages"`
	PreferredBrandIndustries pq.StringArray `gorm:"type:text[]" db:"preferred_brand_industries" json:"preferred_brand_industries"`
	PostType                 pq.StringArray `gorm:"type:text[]" db:"post_type" json:"post_type"`
	CollaborationType        pq.StringArray `gorm:"type:text[]" db:"collaboration_type" json:"collaboration_type"`

	FollowerCount        int `gorm:"default:0" db:"follower_count" json:"follower_count"`
	ReachCount           int `gorm:"default:0" db:"reach_count" json:"reach_count"`
	InteractionCount     int `gorm:"default:0" db:"interaction_count" json:"interaction_count"`
	CompletionPercentage int `gorm:"default:0" db:"completion_percentage" json:"completion_percentage"`

	PrimarySocial string `gorm:"type:varchar(255)" db:"primary_social" json:"primary_social"`
	SocialType    string `gorm:"type:varchar(50)" db:"social_type" json:"social_type"`

	CreationTime int64 `gorm:"type:bigint" db:"creation_time" json:"creation_time"`
	LastUseTime  int64 `gorm:"type:bigint" db:"last_use_time" json:"last_use_time"`

	EstimatedGender string `gorm:"type:varchar(50)" db:"estimated_gender" json:"estimated_gender"`
}

// TableName specifies the table name for GORM
func (Influencers) TableName() string {
	return InfluencersTableName
}

// Insert creates or updates an influencer
func (data *Influencers) Insert() error {
	return rdb.GormDB.Save(data).Error
}

// InsertMultiple bulk inserts or updates multiple influencers
func (_ Influencers) InsertMultiple(influencers []Influencers) error {
	return rdb.GormDB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(influencers, 100).Error
}

// DeleteMultiple deletes multiple influencers by their IDs
func (_ Influencers) DeleteMultiple(influencers []Influencers) error {
	ids := extractInfluencerIDs(influencers)
	if len(ids) == 0 {
		return nil
	}
	return rdb.GormDB.Where("id IN ?", ids).Delete(&Influencers{}).Error
}

// Get retrieves a single influencer by ID
func (data *Influencers) Get(id string) error {
	err := rdb.GormDB.Where("id = ?", id).First(data).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("influencer not found")
		}
		return err
	}
	return nil
}

// GetMultiple retrieves multiple influencers by IDs
func (_ Influencers) GetMultiple(ids []string) ([]Influencers, error) {
	var results []Influencers
	err := rdb.GormDB.Where("id IN ?", ids).Find(&results).Error
	return results, err
}

// GetPaginated retrieves paginated influencers
func (_ Influencers) GetPaginated(offset, limit int) ([]Influencers, error) {
	var results []Influencers
	err := rdb.GormDB.
		Order("creation_time DESC").
		Limit(limit).
		Offset(offset).
		Find(&results).Error

	return results, err
}

// GetExploreInfluencerIDs fetches influencer IDs for brand explore flow.
func (_ Influencers) GetExploreInfluencerIDs(locations, categories, languages []string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 100
	}

	db := rdb.GormDB.Model(&Influencers{}).
		Where("completion_percentage >= ?", 60).
		Where("id NOT IN ?", excludedInfluencerIDs)

	if len(locations) > 0 {
		db = db.Where("location IN ?", locations)
	}
	if len(categories) > 0 {
		db = db.Where("categories && ?", pq.StringArray(categories))
	}
	if len(languages) > 0 {
		db = db.Where("languages && ?", pq.StringArray(languages))
	}

	ids := []string{}
	err := db.
		Order("CASE WHEN reach_count > 20000 AND follower_count > 5000 THEN 1 ELSE 0 END DESC").
		Order("last_use_time DESC").
		Limit(limit).
		Pluck("id", &ids).Error
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// GetInfluencerForInfluencerIDs fetches influencer IDs for i2i flow.
func (_ Influencers) GetInfluencerForInfluencerIDs(location string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 100
	}

	db := rdb.GormDB.Model(&Influencers{}).
		Where("completion_percentage >= ?", 60).
		Where("id NOT IN ?", excludedInfluencerIDs)

	ids := []string{}
	err := db.
		Order(clause.Expr{
			SQL:  "CASE WHEN LOWER(location) = LOWER(?) THEN 100 WHEN random() > 0.95 THEN 100 ELSE 99 END DESC",
			Vars: []interface{}{location},
		}).
		Order("CASE WHEN reach_count > 20000 AND follower_count > 5000 THEN 1 ELSE 0 END DESC").
		Order("last_use_time DESC").
		Limit(limit).
		Pluck("id", &ids).Error
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// Update updates specific fields of an influencer
func (data *Influencers) Update(updates map[string]interface{}) error {
	return rdb.GormDB.Model(data).Updates(updates).Error
}

// Delete deletes an influencer
func (data *Influencers) Delete() error {
	return rdb.GormDB.Delete(data).Error
}

// Count returns the total number of influencers
func (Influencers) Count() (int64, error) {
	var count int64
	err := rdb.GormDB.Model(&Influencers{}).Count(&count).Error
	return count, err
}

// extractInfluencerIDs extracts IDs from a slice of Influencers
func extractInfluencerIDs(data []Influencers) []string {
	ids := []string{}
	for _, d := range data {
		ids = append(ids, d.ID)
	}
	return ids
}
