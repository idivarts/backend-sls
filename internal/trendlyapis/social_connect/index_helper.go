package social_connect

import (
	"log"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

// upsertSocialIndex writes a SocialAccountIndex entry mapping a platform account
// id (IG Business Account id / FB Page id) back to the Trendly entity + social
// account that connected it. The inbox webhook router reads this to resolve an
// incoming Meta event (keyed by entry.id) in O(1).
//
// Best-effort: a failure is logged but never fails the OAuth callback, since the
// connection itself succeeded — only real-time webhook routing degrades.
func upsertSocialIndex(platformAccountID string, platform trendlymodels.Platform, state *OAuthState, socialID string, now int64) {
	if platformAccountID == "" {
		return
	}
	idx := &trendlymodels.SocialAccountIndex{
		PlatformAccountID: platformAccountID,
		Platform:          platform,
		App:               state.App,
		SocialID:          socialID,
		UpdatedAt:         now,
	}
	if state.BrandID != "" {
		idx.BrandID = state.BrandID
	} else {
		idx.UserID = state.UserID
	}
	if _, err := idx.Set(); err != nil {
		log.Printf("social_connect: failed to upsert social index for %s: %v", platformAccountID, err)
	}
}
