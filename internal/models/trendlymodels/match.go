package trendlymodels

import (
	"fmt"

	"cloud.google.com/go/bigquery"
	"github.com/idivarts/backend-sls/pkg/myquery"
)

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

func (data BQInfluencers) GetInsertSQL(table string) (*bigquery.Query, error) {
	sql := "INSERT INTO `" + table + "` (categories, collaboration_type, completion_percentage, follower_count, id, interaction_count, languages, location, post_type, preferred_brand_industries, primary_social, reach_count, social_type) VALUES (@categories, @collaboration_type, @completion_percentage, @follower_count, @id, @interaction_count, @languages, @location, @post_type, @preferred_brand_industries, @primary_social, @reach_count, @social_type)"

	query := myquery.Client.Query(sql)

	query.Parameters = []bigquery.QueryParameter{
		{Name: "categories", Value: data.Categories},
		{Name: "collaboration_type", Value: data.CollaborationType},
		{Name: "completion_percentage", Value: data.CompletionPercentage},
		{Name: "follower_count", Value: data.FollowerCount},
		{Name: "id", Value: data.ID},
		{Name: "interaction_count", Value: data.InteractionCount},
		{Name: "languages", Value: data.Languages},
		{Name: "location", Value: data.Location},
		{Name: "post_type", Value: data.PostType},
		{Name: "preferred_brand_industries", Value: data.PreferredBrandIndustries},
		{Name: "primary_social", Value: data.PrimarySocial},
		{Name: "reach_count", Value: data.ReachCount},
		{Name: "social_type", Value: data.SocialType},
	}

	return query, nil
}
func (_ BQInfluencers) GetInsertMultipleSQL(table string, data []BQInfluencers) (*bigquery.Query, error) {
	sql := "INSERT INTO `" + table + "` (categories, collaboration_type, completion_percentage, follower_count, id, interaction_count, languages, location, post_type, preferred_brand_industries, primary_social, reach_count, social_type) VALUES "

	parameters := []bigquery.QueryParameter{}
	valuePlaceholders := ""

	for index, d := range data {
		if index > 0 {
			valuePlaceholders += ", "
		}
		valuePlaceholders += fmt.Sprintf("(@categories_%d, @collaboration_type_%d, @completion_percentage_%d, @follower_count_%d, @id_%d, @interaction_count_%d, @languages_%d, @location_%d, @post_type_%d, @preferred_brand_industries_%d, @primary_social_%d, @reach_count_%d, @social_type_%d)", index, index, index, index, index, index, index, index, index, index, index, index, index)

		parameters = append(parameters,
			bigquery.QueryParameter{Name: fmt.Sprintf("categories_%d", index), Value: d.Categories},
			bigquery.QueryParameter{Name: fmt.Sprintf("collaboration_type_%d", index), Value: d.CollaborationType},
			bigquery.QueryParameter{Name: fmt.Sprintf("completion_percentage_%d", index), Value: d.CompletionPercentage},
			bigquery.QueryParameter{Name: fmt.Sprintf("follower_count_%d", index), Value: d.FollowerCount},
			bigquery.QueryParameter{Name: fmt.Sprintf("id_%d", index), Value: d.ID},
			bigquery.QueryParameter{Name: fmt.Sprintf("interaction_count_%d", index), Value: d.InteractionCount},
			bigquery.QueryParameter{Name: fmt.Sprintf("languages_%d", index), Value: d.Languages},
			bigquery.QueryParameter{Name: fmt.Sprintf("location_%d", index), Value: d.Location},
			bigquery.QueryParameter{Name: fmt.Sprintf("post_type_%d", index), Value: d.PostType},
			bigquery.QueryParameter{Name: fmt.Sprintf("preferred_brand_industries_%d", index), Value: d.PreferredBrandIndustries},
			bigquery.QueryParameter{Name: fmt.Sprintf("primary_social_%d", index), Value: d.PrimarySocial},
			bigquery.QueryParameter{Name: fmt.Sprintf("reach_count_%d", index), Value: d.ReachCount},
			bigquery.QueryParameter{Name: fmt.Sprintf("social_type_%d", index), Value: d.SocialType},
		)
	}

	sql += valuePlaceholders

	query := myquery.Client.Query(sql)
	query.Parameters = parameters

	return query, nil
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
