package trendlymodels

import "github.com/doug-martin/goqu/v9"

type BQInfluencers struct {
	ID string `db:"id" bigquery:"id"`

	Location string `db:"location" bigquery:"location"`

	Categories               []string `db:"categories" bigquery:"categories"`
	Languages                []string `db:"languages" bigquery:"languages"`
	PreferredBrandIndustries []string `db:"preferred_brand_industries" bigquery:"preferred_brand_industries"`
	PostType                 []string `db:"post_type" bigquery:"post_type"`
	CollaborationType        []string `db:"collaboration_type" bigquery:"collaboration_type"`

	FollowerCount        int `db:"follower_count" bigquery:"follower_count"`
	ReachCount           int `db:"reach_count" bigquery:"reach_count"`
	InteractionCount     int `db:"interaction_count" bigquery:"interaction_count"`
	CompletionPercentage int `db:"completion_percentage" bigquery:"completion_percentage"`

	PrimarySocial string `db:"primary_social" bigquery:"primary_social"`
	SocialType    string `db:"social_type" bigquery:"social_type"`
}

type BQInfluencerViews struct {
	InfluencerID string `db:"influencer_id" bigquery:"influencer_id"`
	BrandID      string `db:"brand_id" bigquery:"brand_id"`
	Time         int64  `db:"time" bigquery:"time"`
}

func (data BQInfluencers) GetInsertSQL(table string) (*string, error) {
	ds := goqu.Insert(table).Rows(data)
	sql, _, err := ds.ToSQL()
	if err != nil {
		return nil, err
	}

	return &sql, err
}

func GetMultipleInsertSQL(table string, data []interface{}) (*string, error) {
	ds := goqu.Insert(table).Rows(data...)
	sql, _, err := ds.ToSQL()
	if err != nil {
		return nil, err
	}

	return &sql, err
}

// -------- USER LISTING
// InfluencerCategories | Category (MATCH)
// PreferredLanguages | Languages (MATCH)
// PreferredBrandIndustries | Industries (MATCH)
// Reach (Sorting)
// Followers (Sorting)
// CompletionPercentage (Sorting)

// -------- COLLAB LISTING

// -------- END

// type UserPreferences struct {
// 	BudgetForPaidCollabs
// 	ContentWillingToPost
// 	Goal
// 	MaximumMonthlyCollabs
// 	PreferredBrandIndustries
// 	PreferredCollaborationType
// 	PreferredLanguages
// 	PreferredVideoType
// }

// type BrandPreferences struct {
// 	PromotionType
// 	InfluencerCategories
// 	Languages
// 	Locations
// 	Platforms
// 	CollaborationPostTypes
// 	TimeCommitments
// 	ContentVideoType
// }
