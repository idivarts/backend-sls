package social_connect

import (
	"log"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/internal/socialsync"
)

// CleanupBrandSocialData purges every data surface derived from a single
// disconnected account — inbox (DMs + comments), media, and analytics
// (cache + daily snapshots). Best-effort: the account doc is already gone by
// the time this runs, so partial failures are logged but never returned.
// Called by the social_sqs worker (OpDisconnectCleanup) and by the inline
// fallback when the queue is not configured.
func CleanupBrandSocialData(brandID, socialID string) {
	if n, err := trendlymodels.DeleteInboxConversationsBySocial(brandID, socialID); err != nil {
		log.Printf("social_connect cleanup: inbox cleanup for %s/%s failed after %d: %v", brandID, socialID, n, err)
	}
	if n, err := trendlymodels.DeleteInboxMediaBySocial(brandID, socialID); err != nil {
		log.Printf("social_connect cleanup: media cleanup for %s/%s failed after %d: %v", brandID, socialID, n, err)
	}
	if n, err := trendlymodels.DeleteAnalyticsBySocial(brandID, socialID); err != nil {
		log.Printf("social_connect cleanup: analytics cleanup for %s/%s failed after %d: %v", brandID, socialID, n, err)
	}
}

// enqueueDisconnectCleanup hands the post-disconnect purge to the shared social
// queue so the DELETE handler returns immediately. Falls back to running inline
// when SOCIAL_SYNC_QUEUE_URL is unset (local dev) — same Firestore mutations
// either way, just synchronous.
func enqueueDisconnectCleanup(brandID, socialID string) {
	msg := socialsync.Message{
		Type:     socialsync.OpDisconnectCleanup,
		BrandID:  brandID,
		SocialID: socialID,
	}
	queued, err := socialsync.Enqueue(msg)
	if err != nil {
		log.Printf("social_connect: disconnect cleanup enqueue failed for %s/%s: %v — running inline", brandID, socialID, err)
		CleanupBrandSocialData(brandID, socialID)
		return
	}
	if !queued {
		log.Printf("social_connect: %s unset — running disconnect cleanup inline for %s/%s", socialsync.QueueEnv, brandID, socialID)
		CleanupBrandSocialData(brandID, socialID)
	}
}
