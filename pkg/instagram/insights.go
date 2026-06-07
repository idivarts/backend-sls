package instagram

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
)

// InsightMetric is a valid value for the `metric` query parameter on the
// Instagram User Insights endpoint.
// Docs: https://developers.facebook.com/docs/instagram-platform/api-reference/instagram-user/insights
type InsightMetric string

const (
	MetricAccountsEngaged             InsightMetric = "accounts_engaged"
	MetricComments                    InsightMetric = "comments"
	MetricEngagedAudienceDemographics InsightMetric = "engaged_audience_demographics"
	MetricFollowsAndUnfollows         InsightMetric = "follows_and_unfollows"
	MetricFollowerDemographics        InsightMetric = "follower_demographics"
	MetricImpressions                 InsightMetric = "impressions" // deprecated v22.0+
	MetricLikes                       InsightMetric = "likes"
	MetricProfileLinksTaps            InsightMetric = "profile_links_taps"
	MetricReach                       InsightMetric = "reach"
	MetricReplies                     InsightMetric = "replies"
	MetricReposts                     InsightMetric = "reposts"
	MetricSaves                       InsightMetric = "saves"
	MetricShares                      InsightMetric = "shares"
	MetricTotalInteractions           InsightMetric = "total_interactions"
	MetricViews                       InsightMetric = "views"
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

// ─── Response types ───────────────────────────────────────────────────────────
//
// The IG insights endpoint returns one of two shapes per metric depending on
// metric_type:
//   - time_series : a `values` array of {value, end_time} points
//   - total_value : a single `total_value.value`, optionally with `breakdowns`
//     (used for demographics — follower_demographics / engaged_audience_demographics)

// InsightTimeSeriesValue is one point in a time_series metric.
type InsightTimeSeriesValue struct {
	Value   int64  `json:"value"`
	EndTime string `json:"end_time"`
}

// InsightBreakdownResult is one demographic bucket (e.g. age "18-24" → 1200).
type InsightBreakdownResult struct {
	DimensionValues []string `json:"dimension_values"`
	Value           int64    `json:"value"`
}

// InsightBreakdownGroup groups results under a set of dimension keys (e.g. ["age"]).
type InsightBreakdownGroup struct {
	DimensionKeys []string                 `json:"dimension_keys"`
	Results       []InsightBreakdownResult `json:"results"`
}

// InsightTotalValue is the total_value object — either a scalar `value` or
// `breakdowns` for demographic metrics.
type InsightTotalValue struct {
	Value      int64                   `json:"value"`
	Breakdowns []InsightBreakdownGroup `json:"breakdowns,omitempty"`
}

// InsightDatum is a single metric entry in the response.
type InsightDatum struct {
	Name        string                   `json:"name"`
	Period      string                   `json:"period"`
	Title       string                   `json:"title"`
	Description string                   `json:"description"`
	Values      []InsightTimeSeriesValue `json:"values,omitempty"`
	TotalValue  *InsightTotalValue       `json:"total_value,omitempty"`
	ID          string                   `json:"id"`
}

// InsightResponse is the top-level insights API response.
type InsightResponse struct {
	Data []InsightDatum `json:"data"`
}

// Find returns the datum for the given metric name, or nil if absent.
func (r *InsightResponse) Find(metric InsightMetric) *InsightDatum {
	if r == nil {
		return nil
	}
	for i := range r.Data {
		if r.Data[i].Name == string(metric) {
			return &r.Data[i]
		}
	}
	return nil
}

// Total returns the scalar total for a metric: the total_value if present,
// otherwise the sum of the time series. Returns 0 if the metric is absent.
func (r *InsightResponse) Total(metric InsightMetric) int64 {
	d := r.Find(metric)
	if d == nil {
		return 0
	}
	if d.TotalValue != nil {
		return d.TotalValue.Value
	}
	var sum int64
	for _, v := range d.Values {
		sum += v.Value
	}
	return sum
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
) (*InsightResponse, error) {
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

	data := InsightResponse{}
	if err := json.Unmarshal(body, &data); err != nil {
		log.Printf("instagram.GetInsights: failed to unmarshal response: %v", err)
		return nil, err
	}
	return &data, nil
}
