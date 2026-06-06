package messenger

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
)

// FBInsightMetric is a valid value for the `metric` query parameter on the
// Facebook Page Insights endpoint.
// Docs: https://developers.facebook.com/docs/graph-api/reference/v22.0/insights
type FBInsightMetric string

const (
	// Page engagement & follows
	FBMetricPageTotalActions               FBInsightMetric = "page_total_actions"
	FBMetricPagePostEngagements            FBInsightMetric = "page_post_engagements"
	FBMetricPageFanAddsByPaidNonPaidUnique FBInsightMetric = "page_fan_adds_by_paid_non_paid_unique"
	FBMetricPageLifetimeEngagedFollowers   FBInsightMetric = "page_lifetime_engaged_followers_unique"
	FBMetricPageDailyFollows               FBInsightMetric = "page_daily_follows"
	FBMetricPageDailyFollowsUnique         FBInsightMetric = "page_daily_follows_unique"
	FBMetricPageDailyUnfollowsUnique       FBInsightMetric = "page_daily_unfollows_unique"
	FBMetricPageFollows                    FBInsightMetric = "page_follows"

	// Impressions & reach
	FBMetricPageImpressions               FBInsightMetric = "page_impressions"
	FBMetricPageImpressionsUnique         FBInsightMetric = "page_impressions_unique"
	FBMetricPageImpressionsPaid           FBInsightMetric = "page_impressions_paid"
	FBMetricPageImpressionsPaidUnique     FBInsightMetric = "page_impressions_paid_unique"
	FBMetricPageImpressionsViral          FBInsightMetric = "page_impressions_viral"
	FBMetricPageImpressionsViralUnique    FBInsightMetric = "page_impressions_viral_unique"
	FBMetricPageImpressionsNonviral       FBInsightMetric = "page_impressions_nonviral"
	FBMetricPageImpressionsNonviralUnique FBInsightMetric = "page_impressions_nonviral_unique"
	FBMetricPagePostsImpressions          FBInsightMetric = "page_posts_impressions"
	FBMetricPagePostsImpressionsUnique    FBInsightMetric = "page_posts_impressions_unique"
	FBMetricPagePostsImpressionsPaid      FBInsightMetric = "page_posts_impressions_paid"

	// Views
	FBMetricPageViewsTotal       FBInsightMetric = "page_views_total"
	FBMetricPageMediaView        FBInsightMetric = "page_media_view"
	FBMetricPageTotalMediaUnique FBInsightMetric = "page_total_media_view_unique"

	// Video views
	FBMetricPageVideoViews              FBInsightMetric = "page_video_views"
	FBMetricPageVideoViewsPaid          FBInsightMetric = "page_video_views_paid"
	FBMetricPageVideoViewsOrganic       FBInsightMetric = "page_video_views_organic"
	FBMetricPageVideoViewsAutoplayed    FBInsightMetric = "page_video_views_autoplayed"
	FBMetricPageVideoViewsClickToPlay   FBInsightMetric = "page_video_views_click_to_play"
	FBMetricPageVideoViewsUnique        FBInsightMetric = "page_video_views_unique"
	FBMetricPageVideoRepeatViews        FBInsightMetric = "page_video_repeat_views"
	FBMetricPageVideoComplete30s        FBInsightMetric = "page_video_complete_views_30s"
	FBMetricPageVideoComplete30sPaid    FBInsightMetric = "page_video_complete_views_30s_paid"
	FBMetricPageVideoComplete30sOrganic FBInsightMetric = "page_video_complete_views_30s_organic"
	FBMetricPageVideoComplete30sUnique  FBInsightMetric = "page_video_complete_views_30s_unique"
	FBMetricPageVideoViewTime           FBInsightMetric = "page_video_view_time"

	// Fans / demographics
	FBMetricPageFans                  FBInsightMetric = "page_fans"
	FBMetricPageFansLocale            FBInsightMetric = "page_fans_locale"
	FBMetricPageFansCity              FBInsightMetric = "page_fans_city"
	FBMetricPageFansCountry           FBInsightMetric = "page_fans_country"
	FBMetricPageFanAdds               FBInsightMetric = "page_fan_adds"
	FBMetricPageFanAddsUnique         FBInsightMetric = "page_fan_adds_unique"
	FBMetricPageFanRemoves            FBInsightMetric = "page_fan_removes"
	FBMetricPageFanRemovesUnique      FBInsightMetric = "page_fan_removes_unique"
	FBMetricPageFansByLikeSource      FBInsightMetric = "page_fans_by_like_source"
	FBMetricPageFansByLikeSourceUnique FBInsightMetric = "page_fans_by_like_source_unique"

	// Reactions
	FBMetricPageReactionsLikeTotal  FBInsightMetric = "page_actions_post_reactions_like_total"
	FBMetricPageReactionsLoveTotal  FBInsightMetric = "page_actions_post_reactions_love_total"
	FBMetricPageReactionsWowTotal   FBInsightMetric = "page_actions_post_reactions_wow_total"
	FBMetricPageReactionsHahaTotal  FBInsightMetric = "page_actions_post_reactions_haha_total"
	FBMetricPageReactionsSorryTotal FBInsightMetric = "page_actions_post_reactions_sorry_total"
	FBMetricPageReactionsAngerTotal FBInsightMetric = "page_actions_post_reactions_anger_total"
	FBMetricPageReactionsTotal      FBInsightMetric = "page_actions_post_reactions_total"

	// Post-level
	FBMetricPostClicks                  FBInsightMetric = "post_clicks"
	FBMetricPostClicksByType            FBInsightMetric = "post_clicks_by_type"
	FBMetricPostImpressions             FBInsightMetric = "post_impressions"
	FBMetricPostImpressionsUnique       FBInsightMetric = "post_impressions_unique"
	FBMetricPostImpressionsPaid         FBInsightMetric = "post_impressions_paid"
	FBMetricPostImpressionsPaidUnique   FBInsightMetric = "post_impressions_paid_unique"
	FBMetricPostImpressionsFan          FBInsightMetric = "post_impressions_fan"
	FBMetricPostImpressionsFanUnique    FBInsightMetric = "post_impressions_fan_unique"
	FBMetricPostImpressionsOrganic      FBInsightMetric = "post_impressions_organic"
	FBMetricPostImpressionsOrganicUnique FBInsightMetric = "post_impressions_organic_unique"
	FBMetricPostImpressionsViral        FBInsightMetric = "post_impressions_viral"
	FBMetricPostImpressionsViralUnique  FBInsightMetric = "post_impressions_viral_unique"
	FBMetricPostReactionsLikeTotal      FBInsightMetric = "post_reactions_like_total"
	FBMetricPostReactionsLoveTotal      FBInsightMetric = "post_reactions_love_total"
	FBMetricPostReactionsWowTotal       FBInsightMetric = "post_reactions_wow_total"
	FBMetricPostReactionsHahaTotal      FBInsightMetric = "post_reactions_haha_total"
	FBMetricPostReactionsSorryTotal     FBInsightMetric = "post_reactions_sorry_total"
	FBMetricPostReactionsAngerTotal     FBInsightMetric = "post_reactions_anger_total"
	FBMetricPostReactionsByTypeTotal    FBInsightMetric = "post_reactions_by_type_total"

	// Post video
	FBMetricPostVideoViews        FBInsightMetric = "post_video_views"
	FBMetricPostVideoViewsUnique  FBInsightMetric = "post_video_views_unique"
	FBMetricPostVideoViewsPaid    FBInsightMetric = "post_video_views_paid"
	FBMetricPostVideoViewsOrganic FBInsightMetric = "post_video_views_organic"
	FBMetricPostVideoAvgTime      FBInsightMetric = "post_video_avg_time_watched"
	FBMetricPostVideoViewTime     FBInsightMetric = "post_video_view_time"
	FBMetricPostVideoLength       FBInsightMetric = "post_video_length"
)

// FBInsightPeriod is a valid value for the `period` query parameter.
type FBInsightPeriod string

const (
	FBPeriodDay            FBInsightPeriod = "day"
	FBPeriodWeek           FBInsightPeriod = "week"
	FBPeriodDays28         FBInsightPeriod = "days_28"
	FBPeriodMonth          FBInsightPeriod = "month"
	FBPeriodLifetime       FBInsightPeriod = "lifetime"
	FBPeriodTotalOverRange FBInsightPeriod = "total_over_range"
)

// FBInsightDatePreset is a valid value for the `date_preset` query parameter.
type FBInsightDatePreset string

const (
	FBDatePresetToday             FBInsightDatePreset = "today"
	FBDatePresetYesterday         FBInsightDatePreset = "yesterday"
	FBDatePresetThisMonth         FBInsightDatePreset = "this_month"
	FBDatePresetLastMonth         FBInsightDatePreset = "last_month"
	FBDatePresetThisQuarter       FBInsightDatePreset = "this_quarter"
	FBDatePresetMaximum           FBInsightDatePreset = "maximum"
	FBDatePresetDataMaximum       FBInsightDatePreset = "data_maximum"
	FBDatePresetLast3d            FBInsightDatePreset = "last_3d"
	FBDatePresetLast7d            FBInsightDatePreset = "last_7d"
	FBDatePresetLast14d           FBInsightDatePreset = "last_14d"
	FBDatePresetLast28d           FBInsightDatePreset = "last_28d"
	FBDatePresetLast30d           FBInsightDatePreset = "last_30d"
	FBDatePresetLast90d           FBInsightDatePreset = "last_90d"
	FBDatePresetLastWeekMonSun    FBInsightDatePreset = "last_week_mon_sun"
	FBDatePresetLastWeekSunSat    FBInsightDatePreset = "last_week_sun_sat"
	FBDatePresetLastQuarter       FBInsightDatePreset = "last_quarter"
	FBDatePresetLastYear          FBInsightDatePreset = "last_year"
	FBDatePresetThisWeekMonToday  FBInsightDatePreset = "this_week_mon_today"
	FBDatePresetThisWeekSunToday  FBInsightDatePreset = "this_week_sun_today"
	FBDatePresetThisYear          FBInsightDatePreset = "this_year"
)

type FBInsightParams struct {
	DatePreset FBInsightDatePreset
	Since      string
	Until      string
}

// joinFBMetrics joins a slice of metrics with commas
func joinFBMetrics(items []FBInsightMetric) string {
	s := make([]string, len(items))
	for i, m := range items {
		s[i] = string(m)
	}
	return strings.Join(s, ",")
}

func GetFacebookInsights(
	pageID,
	accessToken string,
	metrics []FBInsightMetric,
	period FBInsightPeriod,
	params FBInsightParams,
) error {
	// Set up the HTTP client
	client := http.Client{}

	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/%s/%s/insights", BaseURL, ApiVersion, pageID)
	// Create query parameters
	iParam := url.Values{}
	iParam.Set("metric", joinFBMetrics(metrics))
	iParam.Set("period", string(period))
	if params.DatePreset != "" {
		iParam.Set("date_preset", string(params.DatePreset))
	}
	if params.Since != "" {
		iParam.Set("since", params.Since)
	}
	if params.Until != "" {
		iParam.Set("until", params.Until)
	}
	iParam.Set("access_token", accessToken)

	allParams := iParam.Encode()
	log.Println("All Params:", allParams)

	// Combine base URL and query parameters
	apiURL = fmt.Sprintf("%s?%s", apiURL, allParams)

	// Make the API request
	resp, err := client.Get(apiURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return errors.New("Error: Unexpected status code - " + resp.Status + "\n" + string(body))
	}

	// Print the response body
	fmt.Println(string(body))
	return nil
}
