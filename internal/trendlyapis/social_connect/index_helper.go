package social_connect

import (
	"log"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

// upsertSocialIndex adds this Trendly entity as an owner of a platform account
// (IG Business Account id / FB Page id) in the routing index. The inbox webhook
// router reads this to resolve an incoming Meta event (keyed by entry.id) and
// fan out to every owner. Multiple brands may own the same account, so this adds
// (not overwrites) the owner.
//
// Best-effort: a failure is logged but never fails the OAuth callback, since the
// connection itself succeeded — only real-time webhook routing degrades.
func upsertSocialIndex(platformAccountID string, platform trendlymodels.Platform, state *OAuthState, socialID string, now int64) {
	if platformAccountID == "" {
		return
	}
	owner := trendlymodels.SocialIndexOwner{
		App:       state.App,
		SocialID:  socialID,
		UpdatedAt: now,
	}
	if state.BrandID != "" {
		owner.BrandID = state.BrandID
	} else {
		owner.UserID = state.UserID
	}
	if err := trendlymodels.AddSocialAccountOwner(platformAccountID, platform, owner); err != nil {
		log.Printf("social_connect: failed to add social index owner for %s: %v", platformAccountID, err)
	}
}
