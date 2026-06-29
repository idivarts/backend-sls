package analytics

import (
	"encoding/json"
	"time"

	"github.com/idivarts/backend-sls/internal/constants"
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

	fresh := fetchAccount(brandID, acc, r)

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

// fetchAccount dispatches a live analytics fetch to the right platform client.
func fetchAccount(brandID string, acc trendlymodels.SocialAccount, r Range) AccountAnalytics {
	token, err := trendlymodels.GetBrandSocialTokenForAccount(brandID, &acc)
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
	case trendlymodels.PlatformYouTube:
		return fetchYouTube(acc, token, r)
	case trendlymodels.PlatformLinkedInPage:
		// Analytics are an ORG capability (page/follower/share stats). Personal
		// LinkedIn has no analytics API and falls through to unsupported below.
		return fetchLinkedIn(acc, token, r)
	case trendlymodels.PlatformTwitter:
		return fetchTwitter(acc, token, r)
	case trendlymodels.PlatformReddit:
		if constants.RedditEnabled { // gated — see internal/constants/features.go
			return fetchReddit(acc, token, r)
		}
		a := baseAccount(acc, r)
		a.Supported = false
		return a
	default:
		a := baseAccount(acc, r)
		a.Supported = false
		return a
	}
}

// fetchMetaAccount is retained for the daily snapshot path (Meta accounts only).
// It now delegates to the unified dispatcher.
func fetchMetaAccount(brandID string, acc trendlymodels.SocialAccount, r Range) AccountAnalytics {
	return fetchAccount(brandID, acc, r)
}

func decodeCache(c *trendlymodels.AnalyticsCacheDoc) (AccountAnalytics, bool) {
	var a AccountAnalytics
	if err := json.Unmarshal([]byte(c.Payload), &a); err != nil {
		return a, false
	}
	return a, true
}
