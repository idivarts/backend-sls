package trendlydiscovery

import (
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/pkg/myquery"
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
}

// Get only the basic details of influencers as per the matches
func GetInfluencers(c *gin.Context) {
	var req InfluencerFilters
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"message": "Invalid Input", "error": err.Error()})
		return
	}

	// To form the SQL as per the filters
	sql := ``
	myquery.Client.Query(sql)

	var data []InfluencerItem

	c.JSON(200, gin.H{"message": "Success", "data": data})
}
