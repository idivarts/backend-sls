package analytics

import (
	"time"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/youtube"
)

// fetchYouTube builds analytics for a connected YouTube channel using the Data
// API (channel stats + video titles) and the Analytics API (daily metrics +
// demographics). Mirrors fetchInstagram's shape.
func fetchYouTube(acc trendlymodels.SocialAccount, token *trendlymodels.SocialToken, r Range) AccountAnalytics {
	out := baseAccount(acc, r)
	out.Supported = true

	channelID, _ := acc.RawProfile["channelId"].(string)
	if channelID == "" || token == nil || token.AccessToken == "" {
		out.Error = "missing youtube channel id or access token"
		return out
	}
	at := token.AccessToken
	start, end := r.StartEndDates(time.Now())

	// Point-in-time channel stats (refresh subscriber/view counts).
	if stats, err := youtube.GetChannelStats(at, channelID); err == nil && stats != nil {
		out.FollowerCount = stats.Subscribers
	}

	// Daily analytics series → views + engagement buckets.
	points, err := youtube.GetChannelAnalytics(at, channelID, start, end)
	if err != nil {
		out.Error = "youtube analytics: " + err.Error()
	}
	views := Metric{Key: BucketViews, Label: "Views"}
	engagement := Metric{Key: BucketEngagement, Label: "Engagement"}
	for _, p := range points {
		views.Series = append(views.Series, MetricPoint{Date: p.Date, Value: p.Views})
		views.Total += p.Views
		eng := p.Likes + p.Comments + p.Shares
		engagement.Series = append(engagement.Series, MetricPoint{Date: p.Date, Value: eng})
		engagement.Total += eng
	}
	if len(points) > 0 {
		views.Available = true
		engagement.Available = true
	}
	out.Metrics[BucketViews] = views
	out.Metrics[BucketEngagement] = engagement
	// YouTube has no direct "reach"/"impressions" equivalent here; leave those
	// buckets absent so the UI marks them unavailable.

	// Demographics: age + gender (viewerPercentage) and top countries (views).
	var demos []DemographicBucket
	if ag, derr := youtube.GetAgeGenderDemographics(at, channelID, start, end); derr == nil {
		demos = append(demos, mapYouTubeDemos(ag)...)
	}
	if country, derr := youtube.GetCountryViews(at, channelID, start, end); derr == nil && country != nil {
		demos = append(demos, mapYouTubeDemos([]youtube.DemoBucket{*country})...)
	}
	out.Demographics = demos

	// Top videos.
	if vids, terr := youtube.GetTopVideos(at, channelID, start, end, topMediaLimit); terr == nil {
		for _, v := range vids {
			out.TopMedia = append(out.TopMedia, TopMedia{
				ID:           v.ID,
				Caption:      v.Title,
				MediaType:    "VIDEO",
				ThumbnailURL: v.ThumbnailURL,
				Permalink:    "https://www.youtube.com/watch?v=" + v.ID,
				Likes:        v.Likes,
				Comments:     v.Comments,
				Engagement:   v.Likes + v.Comments,
			})
		}
	}
	return out
}

func mapYouTubeDemos(in []youtube.DemoBucket) []DemographicBucket {
	out := make([]DemographicBucket, 0, len(in))
	for _, b := range in {
		entries := make([]DemographicEntry, 0, len(b.Entries))
		for _, e := range b.Entries {
			entries = append(entries, DemographicEntry{Label: e.Label, Value: e.Value})
		}
		if len(entries) > 0 {
			out = append(out, DemographicBucket{Dimension: b.Dimension, Entries: entries})
		}
	}
	return out
}
