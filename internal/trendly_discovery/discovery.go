package trendlydiscovery

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/pkg/myquery"
	"google.golang.org/api/iterator"
)

// - followers/engagements/views can be large; int64 is used.
// - engagementRate is expressed as a percentage (e.g., 1.5 for 1.5%).
// - Field JSON tags mirror the TS property names expected by the app.
type InfluencerItem struct {
	UserID         string  `json:"userId"`
	Fullname       string  `json:"fullname"`
	Username       string  `json:"username"`
	URL            string  `json:"url"`
	Picture        string  `json:"picture"`
	Followers      int64   `json:"followers"`
	Views          *int64  `json:"views,omitempty"`
	Engagements    int64   `json:"engagements"`
	EngagementRate float64 `json:"engagementRate"`
}

// InfluencerFilters represents the filter payload coming from the frontend.
// Types are inferred from the intended semantics rather than the TS state strings.
// Min/Max fields are pointers so omitted filters don't appear in JSON (omitempty).
// All counts are non-negative. quality is a whole number 0..100. ER fields are percentages (e.g., 1 => 1%).
type InfluencerFilters struct {
	// Followers range
	FollowerMin *int64 `json:"followerMin,omitempty"` // minimum followers
	FollowerMax *int64 `json:"followerMax,omitempty"` // maximum followers

	// Content/posts count range
	ContentMin *int `json:"contentMin,omitempty"` // minimum content/posts count
	ContentMax *int `json:"contentMax,omitempty"` // maximum content/posts count

	// Estimated monthly views range
	MonthlyViewMin *int64 `json:"monthlyViewMin,omitempty"`
	MonthlyViewMax *int64 `json:"monthlyViewMax,omitempty"`

	// Estimated monthly engagements (likes+comments etc) range
	MonthlyEngagementMin *int64 `json:"monthlyEngagementMin,omitempty"`
	MonthlyEngagementMax *int64 `json:"monthlyEngagementMax,omitempty"`

	// Median/average metrics ranges (counts)
	AvgViewsMin    *int64 `json:"avgViewsMin,omitempty"`
	AvgViewsMax    *int64 `json:"avgViewsMax,omitempty"`
	AvgLikesMin    *int64 `json:"avgLikesMin,omitempty"`
	AvgLikesMax    *int64 `json:"avgLikesMax,omitempty"`
	AvgCommentsMin *int64 `json:"avgCommentsMin,omitempty"`
	AvgCommentsMax *int64 `json:"avgCommentsMax,omitempty"`

	// Quality/aesthetics slider (0..100)
	QualityMin *int `json:"qualityMin,omitempty"`
	QualityMax *int `json:"qualityMax,omitempty"`

	// Engagement rate (%)
	ERMin *float64 `json:"erMin,omitempty"` // e.g., 1.5 => 1.5%
	ERMax *float64 `json:"erMax,omitempty"`

	// Text filters
	DescKeywords []string `json:"descKeywords,omitempty"` // bio keywords (split client-side or server-side)
	Name         *string  `json:"name,omitempty"`

	// Flags
	IsVerified *bool `json:"isVerified,omitempty"`
	HasContact *bool `json:"hasContact,omitempty"`

	// Multi-selects
	Genders           []string `json:"genders,omitempty"`
	SelectedNiches    []string `json:"selectedNiches,omitempty"`
	SelectedLocations []string `json:"selectedLocations,omitempty"`

	// Sorting & pagination
	Sort          string `json:"sort,omitempty"`           // followers | views | engagement | engagement_rate
	SortDirection string `json:"sort_direction,omitempty"` // asc | desc (default: desc)
	Offset        *int   `json:"offset,omitempty"`
	Limit         *int   `json:"limit,omitempty"`
}

func escapeBQString(s string) string {
	// BigQuery escapes single quotes by doubling them
	return strings.ReplaceAll(s, "'", "''")
}

func inStringList(col string, vals []string) string {
	if len(vals) == 0 {
		return ""
	}
	parts := make([]string, 0, len(vals))
	for _, v := range vals {
		parts = append(parts, fmt.Sprintf("'%s'", escapeBQString(v)))
	}
	return fmt.Sprintf("%s IN (%s)", col, strings.Join(parts, ","))
}

func nichesOverlapClause(vals []string) string {
	if len(vals) == 0 {
		return ""
	}
	parts := make([]string, 0, len(vals))
	for _, v := range vals {
		parts = append(parts, fmt.Sprintf("'%s'", escapeBQString(v)))
	}
	return fmt.Sprintf("ARRAY_LENGTH(ARRAY(SELECT 1 FROM UNNEST(niches) n WHERE n IN (%s))) > 0", strings.Join(parts, ","))
}

// func GetInfluencers(c *gin.Context, req InfluencerFilters) {
func GetInfluencers(c *gin.Context) {
	var req InfluencerFilters
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"message": "Invalid Input", "error": err.Error()})
		return
	}

	// Build dynamic WHERE clauses
	conds := []string{}

	if req.FollowerMin != nil {
		conds = append(conds, fmt.Sprintf("follower_count >= %d", *req.FollowerMin))
	}
	if req.FollowerMax != nil {
		conds = append(conds, fmt.Sprintf("follower_count <= %d", *req.FollowerMax))
	}

	if req.ContentMin != nil {
		conds = append(conds, fmt.Sprintf("content_count >= %d", *req.ContentMin))
	}
	if req.ContentMax != nil {
		conds = append(conds, fmt.Sprintf("content_count <= %d", *req.ContentMax))
	}

	if req.MonthlyViewMin != nil {
		conds = append(conds, fmt.Sprintf("views_count >= %d", *req.MonthlyViewMin))
	}
	if req.MonthlyViewMax != nil {
		conds = append(conds, fmt.Sprintf("views_count <= %d", *req.MonthlyViewMax))
	}

	if req.MonthlyEngagementMin != nil {
		conds = append(conds, fmt.Sprintf("engagements_count >= %d", *req.MonthlyEngagementMin))
	}
	if req.MonthlyEngagementMax != nil {
		conds = append(conds, fmt.Sprintf("engagements_count <= %d", *req.MonthlyEngagementMax))
	}

	if req.AvgViewsMin != nil {
		conds = append(conds, fmt.Sprintf("average_views >= %d", *req.AvgViewsMin))
	}
	if req.AvgViewsMax != nil {
		conds = append(conds, fmt.Sprintf("average_views <= %d", *req.AvgViewsMax))
	}
	if req.AvgLikesMin != nil {
		conds = append(conds, fmt.Sprintf("average_likes >= %d", *req.AvgLikesMin))
	}
	if req.AvgLikesMax != nil {
		conds = append(conds, fmt.Sprintf("average_likes <= %d", *req.AvgLikesMax))
	}
	if req.AvgCommentsMin != nil {
		conds = append(conds, fmt.Sprintf("average_comments >= %d", *req.AvgCommentsMin))
	}
	if req.AvgCommentsMax != nil {
		conds = append(conds, fmt.Sprintf("average_comments <= %d", *req.AvgCommentsMax))
	}

	if req.QualityMin != nil {
		conds = append(conds, fmt.Sprintf("quality_score >= %d", *req.QualityMin))
	}
	if req.QualityMax != nil {
		conds = append(conds, fmt.Sprintf("quality_score <= %d", *req.QualityMax))
	}

	if req.ERMin != nil {
		conds = append(conds, fmt.Sprintf("engagement_rate >= %f", *req.ERMin))
	}
	if req.ERMax != nil {
		conds = append(conds, fmt.Sprintf("engagement_rate <= %f", *req.ERMax))
	}

	if len(req.DescKeywords) > 0 {
		// Match ANY keyword in bio (case-insensitive)
		ors := make([]string, 0, len(req.DescKeywords))
		for _, kw := range req.DescKeywords {
			kw = strings.TrimSpace(kw)
			if kw == "" {
				continue
			}
			ors = append(ors, fmt.Sprintf("LOWER(bio) LIKE '%%%s%%'", strings.ToLower(escapeBQString(kw))))
		}
		if len(ors) > 0 {
			conds = append(conds, fmt.Sprintf("(%s)", strings.Join(ors, " OR ")))
		}
	}

	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		nm := strings.ToLower(escapeBQString(strings.TrimSpace(*req.Name)))
		conds = append(conds, fmt.Sprintf("(LOWER(name) LIKE '%%%s%%' OR LOWER(username) LIKE '%%%s%%')", nm, nm))
	}

	if req.IsVerified != nil {
		if *req.IsVerified {
			conds = append(conds, "profile_verified = TRUE")
		} else {
			conds = append(conds, "profile_verified = FALSE")
		}
	}

	if req.HasContact != nil {
		if *req.HasContact {
			conds = append(conds, "has_contacts = TRUE")
		} else {
			conds = append(conds, "has_contacts = FALSE")
		}
	}

	if clause := inStringList("gender", req.Genders); clause != "" {
		conds = append(conds, clause)
	}
	if clause := inStringList("location", req.SelectedLocations); clause != "" {
		conds = append(conds, clause)
	}
	if clause := nichesOverlapClause(req.SelectedNiches); clause != "" {
		conds = append(conds, clause)
	}

	// Resolve sorting & pagination (safe defaults + whitelist)
	sortMap := map[string]string{
		"followers":       "follower_count",
		"views":           "views_count",
		"engagement":      "engagements_count",
		"engagements":     "engagements_count",
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
	// pagination defaults
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

	// Assemble SQL
	base := `SELECT
  id AS userId,
  name AS fullname,
  username,
  CONCAT('https://instagram.com/', username) AS url,
  profile_pic AS picture,
  follower_count AS followers,
  views_count AS views,
  engagements_count AS engagements,
  engagement_rate AS engagementRate
FROM ` + "`trendly-9ab99.matches.socials`" + `
WHERE social_type = 'instagram'`

	if len(conds) > 0 {
		base += "\n  AND " + strings.Join(conds, "\n  AND ")
	}

	// Ordering & pagination
	base += fmt.Sprintf("\nORDER BY %s %s\nLIMIT %d OFFSET %d", sortCol, strings.ToUpper(dir), limit, offset)

	q := myquery.Client.Query(base)
	it, err := q.Read(context.Background())
	if err != nil {
		c.JSON(500, gin.H{"message": "Query failed", "error": err.Error(), "sql": base})
		return
	}

	type bqRow struct {
		UserID         string  `bigquery:"userId"`
		Fullname       string  `bigquery:"fullname"`
		Username       string  `bigquery:"username"`
		URL            string  `bigquery:"url"`
		Picture        string  `bigquery:"picture"`
		Followers      int64   `bigquery:"followers"`
		Views          *int64  `bigquery:"views"`
		Engagements    int64   `bigquery:"engagements"`
		EngagementRate float64 `bigquery:"engagementRate"`
	}

	out := make([]InfluencerItem, 0, 100)
	for {
		var r bqRow
		err := it.Next(&r)
		if err == iterator.Done {
			break
		}
		if err != nil {
			c.JSON(500, gin.H{"message": "Iteration failed", "error": err.Error(), "sql": base})
			return
		}
		out = append(out, InfluencerItem{
			UserID:         r.UserID,
			Fullname:       r.Fullname,
			Username:       r.Username,
			URL:            r.URL,
			Picture:        r.Picture,
			Followers:      r.Followers,
			Views:          r.Views,
			Engagements:    r.Engagements,
			EngagementRate: r.EngagementRate,
		})
	}

	log.Println("Data Processed", out)
	c.JSON(200, gin.H{"message": "Success", "data": out})
}
