package analytics

import (
	"sort"
	"time"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/instagram"
	"github.com/idivarts/backend-sls/pkg/messenger"
)

const topMediaLimit = 6
const mediaScanLimit = 25

// dayFromEndTime converts a Graph API end_time ("2024-01-02T07:00:00+0000")
// into a YYYY-MM-DD date string. Falls back to the leading 10 chars.
// unixOrZero returns the Unix timestamp of a CustomTime, or 0 when unset.
func unixOrZero(ct messenger.CustomTime) int64 {
	if ct.Time.IsZero() {
		return 0
	}
	return ct.Time.Unix()
}

func dayFromEndTime(s string) string {
	if t, err := time.Parse("2006-01-02T15:04:05-0700", s); err == nil {
		return t.Format("2006-01-02")
	}
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

// baseAccount fills the platform-agnostic header fields shared by every account.
func baseAccount(acc trendlymodels.SocialAccount, r Range) AccountAnalytics {
	return AccountAnalytics{
		SocialID:        acc.ID,
		Platform:        acc.Platform,
		Username:        acc.Username,
		DisplayName:     acc.DisplayName,
		ProfileImageURL: acc.ProfileImageURL,
		FollowerCount:   acc.FollowerCount,
		Metrics:         map[string]Metric{},
		TopMedia:        []TopMedia{},
		Range:           string(r),
		FetchedAt:       time.Now().Unix(),
	}
}

// ─── Instagram ────────────────────────────────────────────────────────────────

func fetchInstagram(acc trendlymodels.SocialAccount, token *trendlymodels.SocialToken, r Range) AccountAnalytics {
	out := baseAccount(acc, r)
	out.Supported = true

	pageID := acc.PlatformAccountID
	if pageID == "" || token == nil || token.AccessToken == "" {
		out.Error = "missing instagram account id or access token"
		return out
	}
	at := token.AccessToken
	since, until := r.SinceUntil(time.Now())

	// Totals (total_value): reach, views, interactions + their components.
	totals, err := instagram.GetInsights(pageID, at,
		[]instagram.InsightMetric{
			instagram.MetricReach,
			instagram.MetricViews,
			instagram.MetricTotalInteractions,
			instagram.MetricLikes,
			instagram.MetricComments,
			instagram.MetricShares,
			instagram.MetricSaves,
		},
		instagram.PeriodDay,
		instagram.InsightParams{MetricType: instagram.MetricTypeTotalValue, StartTime: since, StopTime: until},
	)
	if err != nil {
		// Whole-account insights failed — still return follower count + media below.
		out.Error = "instagram insights: " + err.Error()
	}

	// Time series for the reach + views graphs.
	series, _ := instagram.GetInsights(pageID, at,
		[]instagram.InsightMetric{instagram.MetricReach, instagram.MetricViews},
		instagram.PeriodDay,
		instagram.InsightParams{MetricType: instagram.MetricTypeTimeSeries, StartTime: since, StopTime: until},
	)

	out.Metrics[BucketReach] = igMetric(BucketReach, "Reach", totals, series, instagram.MetricReach)
	out.Metrics[BucketViews] = igMetric(BucketViews, "Views", totals, series, instagram.MetricViews)
	if totals != nil {
		out.Metrics[BucketEngagement] = Metric{
			Key:       BucketEngagement,
			Label:     "Engagement",
			Total:     totals.Total(instagram.MetricTotalInteractions),
			Available: true,
		}
	}

	out.Demographics = instagramDemographics(pageID, at)
	out.TopMedia = instagramTopMedia(pageID, at)
	return out
}

// igMetric builds a normalized Metric, taking the total from the total_value
// call and the series from the time_series call.
func igMetric(key, label string, totals, series *instagram.InsightResponse, m instagram.InsightMetric) Metric {
	metric := Metric{Key: key, Label: label}
	if totals != nil {
		metric.Total = totals.Total(m)
		metric.Available = true
	}
	if series != nil {
		if d := series.Find(m); d != nil {
			for _, v := range d.Values {
				metric.Series = append(metric.Series, MetricPoint{Date: dayFromEndTime(v.EndTime), Value: v.Value})
			}
			metric.Available = true
		}
	}
	return metric
}

// instagramDemographics fetches follower breakdowns best-effort (errors ignored).
func instagramDemographics(pageID, at string) []DemographicBucket {
	var buckets []DemographicBucket
	dims := []struct {
		dimension string
		breakdown instagram.InsightBreakdown
	}{
		{"age", instagram.BreakdownAge},
		{"gender", instagram.BreakdownGender},
		{"country", instagram.BreakdownCountry},
	}
	for _, d := range dims {
		resp, err := instagram.GetInsights(pageID, at,
			[]instagram.InsightMetric{instagram.MetricFollowerDemographics},
			instagram.PeriodLifetime,
			instagram.InsightParams{
				MetricType: instagram.MetricTypeTotalValue,
				Timeframe:  instagram.TimeframeThisMonth,
				Breakdown:  d.breakdown,
			},
		)
		if err != nil || resp == nil {
			continue
		}
		datum := resp.Find(instagram.MetricFollowerDemographics)
		if datum == nil || datum.TotalValue == nil {
			continue
		}
		var entries []DemographicEntry
		for _, grp := range datum.TotalValue.Breakdowns {
			for _, res := range grp.Results {
				if len(res.DimensionValues) == 0 {
					continue
				}
				entries = append(entries, DemographicEntry{Label: res.DimensionValues[0], Value: res.Value})
			}
		}
		if len(entries) > 0 {
			sortEntriesDesc(entries)
			buckets = append(buckets, DemographicBucket{Dimension: d.dimension, Entries: entries})
		}
	}
	return buckets
}

// instagramTopMedia fetches recent media and returns the top performers by engagement.
func instagramTopMedia(pageID, at string) []TopMedia {
	medias, err := instagram.GetMedia(pageID, at, instagram.IGetMediaParams{
		GraphType: 1, // instagram graph
		PageID:    pageID,
		Count:     mediaScanLimit,
	})
	if err != nil {
		return []TopMedia{}
	}
	out := make([]TopMedia, 0, len(medias))
	for _, m := range medias {
		eng := int64(m.LikeCount + m.CommentsCount)
		out = append(out, TopMedia{
			ID:           m.ID,
			Caption:      m.Caption,
			MediaType:    m.MediaType,
			MediaURL:     m.MediaURL,
			ThumbnailURL: m.ThumbnailURL,
			Permalink:    m.Permalink,
			Timestamp:    unixOrZero(m.Timestamp),
			Likes:        int64(m.LikeCount),
			Comments:     int64(m.CommentsCount),
			Engagement:   eng,
		})
	}
	return topByEngagement(out)
}

// ─── Facebook ─────────────────────────────────────────────────────────────────

func fetchFacebook(acc trendlymodels.SocialAccount, token *trendlymodels.SocialToken, r Range) AccountAnalytics {
	out := baseAccount(acc, r)
	out.Supported = true

	pageID := acc.PlatformAccountID
	if pageID == "" || token == nil || token.AccessToken == "" {
		out.Error = "missing facebook page id or access token"
		return out
	}
	at := token.AccessToken
	preset := r.FBDatePreset()

	// Daily series: impressions (≈ views), unique impressions (≈ reach), engagement.
	resp, err := messenger.GetFacebookInsights(pageID, at,
		[]messenger.FBInsightMetric{
			messenger.FBMetricPageImpressions,
			messenger.FBMetricPageImpressionsUnique,
			messenger.FBMetricPagePostEngagements,
		},
		messenger.FBPeriodDay,
		messenger.FBInsightParams{DatePreset: preset},
	)
	if err != nil {
		out.Error = "facebook insights: " + err.Error()
	}

	out.Metrics[BucketReach] = fbMetric(BucketReach, "Reach", resp, messenger.FBMetricPageImpressionsUnique)
	out.Metrics[BucketImpressions] = fbMetric(BucketImpressions, "Impressions", resp, messenger.FBMetricPageImpressions)
	out.Metrics[BucketEngagement] = fbMetric(BucketEngagement, "Engagement", resp, messenger.FBMetricPagePostEngagements)

	out.Demographics = facebookDemographics(pageID, at)
	out.TopMedia = facebookTopMedia(pageID, at)
	return out
}

// fbMetric builds a normalized Metric from a FB daily series.
func fbMetric(key, label string, resp *messenger.FBInsightResponse, m messenger.FBInsightMetric) Metric {
	metric := Metric{Key: key, Label: label}
	if resp == nil {
		return metric
	}
	d := resp.Find(m)
	if d == nil {
		return metric
	}
	for _, v := range d.Values {
		if n, ok := v.AsInt(); ok {
			metric.Series = append(metric.Series, MetricPoint{Date: dayFromEndTime(v.EndTime), Value: n})
			metric.Total += n
		}
	}
	metric.Available = true
	return metric
}

// facebookDemographics fetches lifetime fan-country breakdown best-effort.
func facebookDemographics(pageID, at string) []DemographicBucket {
	resp, err := messenger.GetFacebookInsights(pageID, at,
		[]messenger.FBInsightMetric{messenger.FBMetricPageFansCountry},
		messenger.FBPeriodLifetime,
		messenger.FBInsightParams{},
	)
	if err != nil || resp == nil {
		return nil
	}
	m := resp.LatestMap(messenger.FBMetricPageFansCountry)
	if len(m) == 0 {
		return nil
	}
	entries := make([]DemographicEntry, 0, len(m))
	for k, v := range m {
		entries = append(entries, DemographicEntry{Label: k, Value: v})
	}
	sortEntriesDesc(entries)
	return []DemographicBucket{{Dimension: "country", Entries: entries}}
}

// facebookTopMedia fetches recent posts with engagement and ranks the best.
func facebookTopMedia(pageID, at string) []TopMedia {
	posts, err := messenger.GetPosts(pageID, at, messenger.IFBPostsParams{
		Count:          mediaScanLimit,
		WithEngagement: true,
	})
	if err != nil {
		return []TopMedia{}
	}
	out := make([]TopMedia, 0, len(posts))
	for _, p := range posts {
		likes := int64(p.LikeCount())
		comments := int64(p.CommentCount())
		eng := likes + comments + int64(p.ShareCount())
		out = append(out, TopMedia{
			ID:           p.ID,
			Caption:      p.Message,
			MediaType:    "POST",
			ThumbnailURL: p.FullPicture,
			Permalink:    p.PermalinkURL,
			Timestamp:    unixOrZero(p.CreatedTime),
			Likes:        likes,
			Comments:     comments,
			Engagement:   eng,
		})
	}
	return topByEngagement(out)
}

// ─── Shared helpers ───────────────────────────────────────────────────────────

func topByEngagement(media []TopMedia) []TopMedia {
	sort.SliceStable(media, func(i, j int) bool {
		return media[i].Engagement > media[j].Engagement
	})
	if len(media) > topMediaLimit {
		media = media[:topMediaLimit]
	}
	return media
}

func sortEntriesDesc(entries []DemographicEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Value > entries[j].Value
	})
}
