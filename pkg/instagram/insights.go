package instagram

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/idivarts/backend-sls/pkg/messenger"
)

// InsightMetric is a valid value for the `metric` query parameter on the
// Instagram User Insights endpoint.
// Docs: https://developers.facebook.com/docs/instagram-platform/api-reference/instagram-user/insights
type InsightMetric string

const (
	MetricAccountsEngaged              InsightMetric = "accounts_engaged"
	MetricComments                     InsightMetric = "comments"
	MetricEngagedAudienceDemographics  InsightMetric = "engaged_audience_demographics"
	MetricFollowsAndUnfollows          InsightMetric = "follows_and_unfollows"
	MetricFollowerDemographics         InsightMetric = "follower_demographics"
	MetricImpressions                  InsightMetric = "impressions" // deprecated v22.0+
	MetricLikes                        InsightMetric = "likes"
	MetricProfileLinksTaps             InsightMetric = "profile_links_taps"
	MetricReach                        InsightMetric = "reach"
	MetricReplies                      InsightMetric = "replies"
	MetricReposts                      InsightMetric = "reposts"
	MetricSaves                        InsightMetric = "saves"
	MetricShares                       InsightMetric = "shares"
	MetricTotalInteractions            InsightMetric = "total_interactions"
	MetricViews                        InsightMetric = "views"
)

// InsightPeriod is a valid value for the `period` query parameter.
type InsightPeriod string

const (
	PeriodDay      InsightPeriod = "day"
	PeriodLifetime InsightPeriod = "lifetime"
)

// InsightMetricType is a valid value for the `metric_type` query parameter.
type InsightMetricType string

const (
	MetricTypeTimeSeries InsightMetricType = "time_series"
	MetricTypeTotalValue InsightMetricType = "total_value"
)

// InsightTimeframe is a valid value for the `timeframe` query parameter.
// Note: last_14_days, last_30_days, last_90_days, and prev_month are
// unsupported from v20.0 onward.
type InsightTimeframe string

const (
	TimeframeLast14Days InsightTimeframe = "last_14_days"
	TimeframeLast30Days InsightTimeframe = "last_30_days"
	TimeframeLast90Days InsightTimeframe = "last_90_days"
	TimeframePrevMonth  InsightTimeframe = "prev_month"
	TimeframeThisMonth  InsightTimeframe = "this_month"
	TimeframeThisWeek   InsightTimeframe = "this_week"
)

// InsightBreakdown is a valid value for the `breakdown` query parameter.
type InsightBreakdown string

const (
	BreakdownContactButtonType InsightBreakdown = "contact_button_type"
	BreakdownFollowType        InsightBreakdown = "follow_type"
	BreakdownMediaProductType  InsightBreakdown = "media_product_type"
	BreakdownAge               InsightBreakdown = "age"
	BreakdownCity              InsightBreakdown = "city"
	BreakdownCountry           InsightBreakdown = "country"
	BreakdownGender            InsightBreakdown = "gender"
)

type InsightParams struct {
	Timeframe  InsightTimeframe
	MetricType InsightMetricType
	Breakdown  InsightBreakdown
	StartTime  string
	StopTime   string
}

// joinMetrics joins a slice of metrics with commas
func joinMetrics(items []InsightMetric) string {
	s := make([]string, len(items))
	for i, m := range items {
		s[i] = string(m)
	}
	return strings.Join(s, ",")
}

func GetInsights(
	pageID,
	accessToken string,
	metrics []InsightMetric,
	period InsightPeriod,
	params InsightParams,
) (*messenger.InstagramBriefProfile, error) {
	// Set up the HTTP client
	client := http.Client{}

	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/%s/%s/insights", BaseURL, ApiVersion, pageID)
	// Create query parameters
	iParam := url.Values{}
	iParam.Set("metric", joinMetrics(metrics))
	iParam.Set("period", string(period))
	if params.Timeframe != "" {
		iParam.Set("timeframe", string(params.Timeframe))
	}
	if params.MetricType != "" {
		iParam.Set("metric_type", string(params.MetricType))
	}
	if params.Breakdown != "" {
		iParam.Set("breakdown", string(params.Breakdown))
	}
	if params.StartTime != "" {
		iParam.Set("since", params.StartTime)
	}
	if params.StopTime != "" {
		iParam.Set("until", params.StopTime)
	}
	iParam.Set("access_token", accessToken)

	allParams := iParam.Encode()
	log.Println("All Params:", allParams)

	// Combine base URL and query parameters
	apiURL = fmt.Sprintf("%s?%s", apiURL, allParams)

	// Make the API request
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("Error: Unexpected status code - " + resp.Status + "\n" + string(body))
	}

	// Print the response body
	fmt.Println(string(body))
	// data := messenger.InstagramBriefProfile{}
	// err = json.Unmarshal(body, &data)
	// if err != nil {
	// 	return nil, err
	// }
	return nil, nil
}
