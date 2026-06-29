package analytics

import (
	"time"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/linkedin"
)

// fetchLinkedIn builds analytics for a connected LinkedIn Company Page using the
// Community Management API (follower + share statistics). Organization-only:
// member connections (no administered org) return Supported=false.
func fetchLinkedIn(acc trendlymodels.SocialAccount, token *trendlymodels.SocialToken, r Range) AccountAnalytics {
	out := baseAccount(acc, r)

	// Org URN from the page account (stored at connect, or derived from the org id).
	orgURN, _ := acc.RawProfile["orgUrn"].(string)
	if orgURN == "" && acc.PlatformAccountID != "" {
		orgURN = "urn:li:organization:" + acc.PlatformAccountID
	}
	if orgURN == "" {
		out.Supported = false
		out.Error = "LinkedIn Page analytics require a Company Page connection"
		return out
	}
	if token == nil || token.AccessToken == "" {
		out.Supported = true
		out.Error = "missing linkedin access token"
		return out
	}
	out.Supported = true
	at := token.AccessToken

	// Follower statistics (total + demographic breakdowns).
	if fs, err := linkedin.GetFollowerStatistics(at, orgURN); err == nil && fs != nil {
		if fs.TotalFollowers > 0 {
			out.FollowerCount = fs.TotalFollowers
		}
		out.Demographics = append(out.Demographics,
			liDemo("industry", fs.ByIndustry),
			liDemo("seniority", fs.BySeniority),
			liDemo("function", fs.ByFunction),
			liDemo("country", fs.ByCountry),
		)
		out.Demographics = nonEmptyDemos(out.Demographics)
	} else if err != nil {
		out.Error = "linkedin follower stats: " + err.Error()
	}

	// Share statistics (impressions / reach / engagement, daily).
	startMs, endMs := r.StartEndMs(time.Now())
	if ss, err := linkedin.GetShareStatistics(at, orgURN, startMs, endMs); err == nil && ss != nil {
		impr := Metric{Key: BucketImpressions, Label: "Impressions", Total: ss.Impressions, Available: true}
		reach := Metric{Key: BucketReach, Label: "Reach", Total: ss.UniqueImpressions, Available: ss.UniqueImpressions > 0}
		eng := Metric{Key: BucketEngagement, Label: "Engagement", Total: ss.Engagement, Available: true}
		for _, d := range ss.Series {
			impr.Series = append(impr.Series, MetricPoint{Date: d.Date, Value: d.Impressions})
			eng.Series = append(eng.Series, MetricPoint{Date: d.Date, Value: d.Engagement})
		}
		out.Metrics[BucketImpressions] = impr
		out.Metrics[BucketReach] = reach
		out.Metrics[BucketEngagement] = eng
	} else if err != nil {
		if out.Error == "" {
			out.Error = "linkedin share stats: " + err.Error()
		}
	}

	return out
}

func liDemo(dimension string, entries []linkedin.StatEntry) DemographicBucket {
	out := DemographicBucket{Dimension: dimension}
	for _, e := range entries {
		out.Entries = append(out.Entries, DemographicEntry{Label: e.Label, Value: e.Value})
	}
	sortEntriesDesc(out.Entries)
	return out
}

func nonEmptyDemos(in []DemographicBucket) []DemographicBucket {
	out := make([]DemographicBucket, 0, len(in))
	for _, b := range in {
		if len(b.Entries) > 0 {
			out = append(out, b)
		}
	}
	return out
}
