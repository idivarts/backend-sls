package analytics

import (
	"sort"
	"time"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/reddit"
)

// fetchReddit builds DERIVED analytics for a connected Reddit account. Reddit has
// no organic analytics API — there is no reach/impressions/views/demographics.
// We surface what listings expose: post score + comment counts (an engagement
// proxy) and the user's top posts. Supported=true but only the engagement bucket
// is populated; the rest are marked unavailable so the UI is honest.
func fetchReddit(acc trendlymodels.SocialAccount, token *trendlymodels.SocialToken, r Range) AccountAnalytics {
	out := baseAccount(acc, r)
	out.Supported = true
	// Reddit accounts have no "followers" — keep FollowerCount at 0 (the connect
	// stores karma in rawProfile; the UI labels Reddit's stat as karma, not added
	// into the cross-account followers total).
	out.FollowerCount = 0

	if token == nil || token.AccessToken == "" {
		out.Error = "missing reddit access token"
		return out
	}

	subs, err := reddit.GetUserSubmissions(token.AccessToken, acc.Username, 100)
	if err != nil {
		out.Error = "reddit analytics: " + err.Error()
		return out
	}

	cutoff := time.Now().AddDate(0, 0, -r.Days())
	type dayAgg struct{ engagement int64 }
	byDay := map[string]*dayAgg{}
	var totalEng int64
	var top []TopMedia
	for _, s := range subs {
		ts := time.Unix(s.CreatedUTC, 0)
		if ts.Before(cutoff) {
			continue
		}
		eng := s.Score + s.NumComments
		totalEng += eng
		day := ts.Format("2006-01-02")
		if byDay[day] == nil {
			byDay[day] = &dayAgg{}
		}
		byDay[day].engagement += eng
		top = append(top, TopMedia{
			ID:         s.ID,
			Caption:    s.Title,
			MediaType:  "POST",
			Permalink:  s.Permalink,
			Timestamp:  s.CreatedUTC,
			Likes:      s.Score,
			Comments:   s.NumComments,
			Engagement: eng,
		})
	}

	days := make([]string, 0, len(byDay))
	for d := range byDay {
		days = append(days, d)
	}
	sort.Strings(days)
	eng := Metric{Key: BucketEngagement, Label: "Engagement (score + comments)", Total: totalEng, Available: true}
	for _, d := range days {
		eng.Series = append(eng.Series, MetricPoint{Date: d, Value: byDay[d].engagement})
	}
	out.Metrics[BucketEngagement] = eng
	out.TopMedia = topByEngagement(top)
	return out
}
