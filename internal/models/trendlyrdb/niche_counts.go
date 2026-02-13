package trendlyrdb

import (
	"github.com/idivarts/backend-sls/internal/openai/deduce"
	"github.com/idivarts/backend-sls/pkg/rdb"
)

const (
	NicheCountsViewName = "niche_counts"
)

// NicheCount represents a row from the niche_counts materialized view
type NicheCount struct {
	Niche           string `gorm:"primaryKey;type:text" db:"niche" json:"niche"`
	AppearanceCount int64  `gorm:"type:bigint" db:"appearance_count" json:"appearance_count"`
}

// TableName specifies the materialized view name for GORM
func (NicheCount) TableName() string {
	return NicheCountsViewName
}

// GetPaginated retrieves paginated niches ordered by appearance count.
// If searchKey is provided (non-empty), it filters niches matching the search key using ILIKE.
func (_ NicheCount) GetPaginated(offset, limit int, searchKey string) ([]NicheCount, error) {
	var results []NicheCount

	query := rdb.GormDB.Order("appearance_count DESC")

	if searchKey != "" {
		query = query.Where("niche ILIKE ?", "%"+searchKey+"%")
	} else {
		query = query.Where("niche IN ?", deduce.AllowedNiches)
	}

	err := query.
		Limit(limit).
		Offset(offset).
		Find(&results).Error

	return results, err
}
