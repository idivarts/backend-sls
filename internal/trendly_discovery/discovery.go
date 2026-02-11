package trendlydiscovery

import (
	"log"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/internal/models/trendlyrdb"
	"github.com/idivarts/backend-sls/pkg/rdb"
	"github.com/lib/pq"
)

// - followers/engagements/views can be large; int64 is used.
// - engagementRate is expressed as a percentage (e.g., 1.5 for 1.5%).
// - Field JSON tags mirror the TS property names expected by the app.
type InfluencerItem struct {
	trendlyrdb.Socials
	IsDiscover bool `json:"isDiscover,omitempty"`
}

type InfluencerInviteUnit struct {
	InfluencerItem
	InvitedAt int64  `json:"invitedAt"`
	Status    string `json:"status"`
}

// InfluencerFilters represents the filter payload coming from the frontend.
// Types are inferred from the intended semantics rather than the TS state strings.
// Min/Max fields are pointers so omitted filters don't appear in JSON (omitempty).
// All counts are non-negative. quality is a whole number 0..100. ER fields are percentages (e.g., 1 => 1%).
type InfluencerFilters = trendlymodels.DiscoverPreferences

// func GetInfluencers(c *gin.Context, req InfluencerFilters) {
func GetInfluencers(c *gin.Context) {
	var req InfluencerFilters
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"message": "Invalid Input", "error": err.Error()})
		return
	}

	socials, err := queryInfluencersFromRDB(req)
	if err != nil {
		c.JSON(500, gin.H{"message": "Query failed", "error": err.Error()})
		return
	}

	out := make([]InfluencerItem, 0, len(socials))
	for _, s := range socials {
		brief := convertSocialsToBreif(s)
		out = append(out, InfluencerItem{Socials: *brief, IsDiscover: true})
	}

	log.Println("Data Processed", out)
	c.JSON(200, gin.H{"message": "Success", "data": out})
}

// queryInfluencersFromRDB builds a GORM query over the Postgres-backed
// socials table, applying the same filters that were previously used
// in the BigQuery-based FormSQL function.
func queryInfluencersFromRDB(req InfluencerFilters) ([]trendlyrdb.Socials, error) {
	db := rdb.GormDB.Model(&trendlyrdb.Socials{}).Where("social_type = ?", "instagram")

	if req.FollowerMin != nil {
		db = db.Where("follower_count >= ?", *req.FollowerMin)
	}
	if req.FollowerMax != nil {
		db = db.Where("follower_count <= ?", *req.FollowerMax)
	}

	if req.ContentMin != nil {
		db = db.Where("content_count >= ?", *req.ContentMin)
	}
	if req.ContentMax != nil {
		db = db.Where("content_count <= ?", *req.ContentMax)
	}

	if req.MonthlyViewMin != nil {
		db = db.Where("views_count >= ?", *req.MonthlyViewMin)
	}
	if req.MonthlyViewMax != nil {
		db = db.Where("views_count <= ?", *req.MonthlyViewMax)
	}

	if req.MonthlyEngagementMin != nil {
		db = db.Where("engagement_count >= ?", *req.MonthlyEngagementMin)
	}
	if req.MonthlyEngagementMax != nil {
		db = db.Where("engagement_count <= ?", *req.MonthlyEngagementMax)
	}

	if req.AvgViewsMin != nil {
		db = db.Where("average_views >= ?", *req.AvgViewsMin)
	}
	if req.AvgViewsMax != nil {
		db = db.Where("average_views <= ?", *req.AvgViewsMax)
	}
	if req.AvgLikesMin != nil {
		db = db.Where("average_likes >= ?", *req.AvgLikesMin)
	}
	if req.AvgLikesMax != nil {
		db = db.Where("average_likes <= ?", *req.AvgLikesMax)
	}
	if req.AvgCommentsMin != nil {
		db = db.Where("average_comments >= ?", *req.AvgCommentsMin)
	}
	if req.AvgCommentsMax != nil {
		db = db.Where("average_comments <= ?", *req.AvgCommentsMax)
	}

	if req.QualityMin != nil {
		db = db.Where("quality_score >= ?", *req.QualityMin)
	}
	if req.QualityMax != nil {
		db = db.Where("quality_score <= ?", *req.QualityMax)
	}

	if req.ERMin != nil {
		db = db.Where("engagement_rate >= ?", *req.ERMin)
	}
	if req.ERMax != nil {
		db = db.Where("engagement_rate <= ?", *req.ERMax)
	}

	if req.SelectedLocations != nil && len(req.SelectedLocations) > 0 {
		if req.DescKeywords == nil {
			req.DescKeywords = make([]string, 0)
		}
		req.DescKeywords = append(req.DescKeywords, req.SelectedLocations...)
	}

	if len(req.DescKeywords) > 0 {
		ors := make([]string, 0, len(req.DescKeywords))
		args := make([]interface{}, 0, len(req.DescKeywords))
		for _, kw := range req.DescKeywords {
			kw = strings.TrimSpace(kw)
			if kw == "" {
				continue
			}
			ors = append(ors, "LOWER(bio) LIKE ?")
			args = append(args, "%"+strings.ToLower(kw)+"%")
		}
		if len(ors) > 0 {
			db = db.Where("("+strings.Join(ors, " OR ")+")", args...)
		}
	} else if len(req.SelectedLocations) > 0 {
		db = db.Where("location IN ?", req.SelectedLocations)
	}

	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		nm := strings.ToLower(strings.TrimSpace(*req.Name))
		db = db.Where("(LOWER(name) LIKE ? OR LOWER(username) LIKE ?)", "%"+nm+"%", "%"+nm+"%")
	}

	if req.IsVerified != nil {
		db = db.Where("profile_verified = ?", *req.IsVerified)
	}

	if req.HasContact != nil {
		db = db.Where("has_contacts = ?", *req.HasContact)
	}

	if len(req.Genders) > 0 {
		db = db.Where("gender IN ?", req.Genders)
	}

	if len(req.SelectedNiches) > 0 {
		db = db.Where("niches && ?", pq.StringArray(req.SelectedNiches))
	}

	sortMap := map[string]string{
		"followers":       "follower_count",
		"views":           "views_count",
		"engagement":      "engagement_count",
		"engagements":     "engagement_count",
		"engagement_rate": "engagement_rate",
		"er":              "engagement_rate",
	}
	sortCol := sortMap[strings.ToLower(strings.TrimSpace(req.Sort))]
	if sortCol == "" {
		sortCol = "follower_count"
	}
	dir := strings.ToLower(strings.TrimSpace(req.SortDirection))
	if dir != "asc" {
		dir = "desc"
	}

	limit := 15
	if req.Limit != nil {
		limit = *req.Limit
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	offset := 0
	if req.Offset != nil && *req.Offset > 0 {
		offset = *req.Offset
	}

	db = db.Order(sortCol + " " + dir).Limit(limit).Offset(offset)

	var results []trendlyrdb.Socials
	if err := db.Find(&results).Error; err != nil {
		return nil, err
	}
	return results, nil
}
