package analytics

import (
	"encoding/json"
	"time"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

// cacheTTLSeconds is how long a live analytics fetch is reused before refetching.
// Short enough to feel fresh, long enough to absorb dashboard reloads and stay
// well under Meta's per-token rate limits.
const cacheTTLSeconds = 3 * 60 * 60 // 3h

// getAccountAnalytics returns analytics for a single connected account.
//
// Strategy: serve from cache when fresh; otherwise fetch live from Meta and
// refresh the cache. If a live fetch fails but a cached copy exists, the cached
// copy is served with Stale=true so the dashboard degrades gracefully rather
// than blanking out.
func getAccountAnalytics(brandID string, acc trendlymodels.SocialAccount, r Range) AccountAnalytics {
	now := time.Now().Unix()

	cached, _ := trendlymodels.GetAnalyticsCache(brandID, acc.ID, string(r))
	if cached != nil && now-cached.FetchedAt < cacheTTLSeconds {
		if a, ok := decodeCache(cached); ok {
			return a
		}
	}

	// Non-Meta platforms aren't wired for analytics in phase 1 — return the
	// header (follower count etc.) flagged unsupported. Not worth caching.
	if acc.Platform != trendlymodels.PlatformInstagram && acc.Platform != trendlymodels.PlatformFacebook {
		a := baseAccount(acc, r)
		a.Supported = false
		return a
	}

	fresh := fetchMetaAccount(brandID, acc, r)

	// Live fetch errored — fall back to any cached copy, marked stale.
	if fresh.Error != "" && cached != nil {
		if a, ok := decodeCache(cached); ok {
			a.Stale = true
			return a
		}
	}

	// Persist a clean result for next time (best-effort).
	if fresh.Error == "" {
		if payload, err := json.Marshal(fresh); err == nil {
			_ = trendlymodels.SetAnalyticsCache(brandID, &trendlymodels.AnalyticsCacheDoc{
				SocialID:  acc.ID,
				Range:     string(r),
				Payload:   string(payload),
				FetchedAt: now,
			})
		}
	}
	return fresh
}

// fetchMetaAccount dispatches a live fetch to the right Meta platform client.
func fetchMetaAccount(brandID string, acc trendlymodels.SocialAccount, r Range) AccountAnalytics {
	token, err := trendlymodels.GetBrandSocialToken(brandID, acc.ID)
	if err != nil {
		a := baseAccount(acc, r)
		a.Supported = true
		a.Error = "missing access token"
		return a
	}
	switch acc.Platform {
	case trendlymodels.PlatformInstagram:
		return fetchInstagram(acc, token, r)
	case trendlymodels.PlatformFacebook:
		return fetchFacebook(acc, token, r)
	default:
		a := baseAccount(acc, r)
		a.Supported = false
		return a
	}
}

func decodeCache(c *trendlymodels.AnalyticsCacheDoc) (AccountAnalytics, bool) {
	var a AccountAnalytics
	if err := json.Unmarshal([]byte(c.Payload), &a); err != nil {
		return a, false
	}
	return a, true
}
