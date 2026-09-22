package analytics

import (
	"sort"
	"time"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/twitter"
)

// fetchTwitter builds analytics for a connected X account by aggregating recent
// own-tweet metrics. X exposes no account-level daily timeseries, so the series
// is derived from per-tweet timestamps. Organic/non-public metrics (impressions)
// are only available for own tweets < 30 days old — the daily snapshot captures
// them before they expire.
func fetchTwitter(acc trendlymodels.SocialAccount, token *trendlymodels.SocialToken, r Range) AccountAnalytics {
	out := baseAccount(acc, r)
	out.Supported = true

	selfID, _ := acc.RawProfile["id"].(string)
	if selfID == "" || token == nil || token.AccessToken == "" {
		out.Error = "missing twitter user id or access token"
		return out
	}
	at := token.AccessToken

	tweets, err := twitter.GetOwnTweetsWithMetrics(at, selfID, 100)
	if err != nil {
		out.Error = "twitter analytics: " + err.Error()
	}

	// Keep only tweets inside the window.
	cutoff := time.Now().AddDate(0, 0, -r.Days())
	type dayAgg struct{ impressions, engagement int64 }
	byDay := map[string]*dayAgg{}
	var totalImpr, totalEng int64
	var top []TopMedia
	for _, t := range tweets {
		if t.CreatedAt.Before(cutoff) {
			continue
		}
		eng := t.Likes + t.Replies + t.Retweets + t.Quotes
		totalImpr += t.Impressions
		totalEng += eng
		day := t.CreatedAt.Format("2006-01-02")
		if byDay[day] == nil {
			byDay[day] = &dayAgg{}
		}
		byDay[day].impressions += t.Impressions
		byDay[day].engagement += eng
		top = append(top, TopMedia{
			ID:         t.ID,
			Caption:    t.Text,
			MediaType:  "TWEET",
			Permalink:  "https://twitter.com/" + acc.Username + "/status/" + t.ID,
			Timestamp:  t.CreatedAt.Unix(),
			Likes:      t.Likes,
			Comments:   t.Replies,
			Engagement: eng,
		})
	}

	// Sorted daily series.
	days := make([]string, 0, len(byDay))
	for d := range byDay {
		days = append(days, d)
	}
	sort.Strings(days)
	impr := Metric{Key: BucketImpressions, Label: "Impressions"}
	eng := Metric{Key: BucketEngagement, Label: "Engagement"}
	for _, d := range days {
		impr.Series = append(impr.Series, MetricPoint{Date: d, Value: byDay[d].impressions})
		eng.Series = append(eng.Series, MetricPoint{Date: d, Value: byDay[d].engagement})
	}
	impr.Total = totalImpr
	impr.Available = totalImpr > 0 // impressions need non-public metrics (own, <30d)
	eng.Total = totalEng
	eng.Available = true
	out.Metrics[BucketImpressions] = impr
	out.Metrics[BucketEngagement] = eng
	// X has no public "reach" metric — leave that bucket absent (UI marks N/A).

	out.TopMedia = topByEngagement(top)
	return out
}
