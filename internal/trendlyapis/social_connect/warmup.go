package social_connect

import (
	"log"

	"github.com/idivarts/backend-sls/internal/socialsync"
)

// enqueueWarmup kicks the brand-wide background fetches (inbox DMs, media,
// analytics) right after a new account connects, so those pages already have
// content on first open instead of a cold load. Best-effort: a queue failure must
// never break the OAuth redirect. The ops are brand-wide (they cover every
// connected account), so this runs once per connect — not once per saved account.
func enqueueWarmup(brandID string) {
	if brandID == "" {
		return // only brand connections have inbox/media/analytics surfaces
	}
	jobs := []socialsync.Message{
		{Type: socialsync.OpInboxSync, BrandID: brandID},
		{Type: socialsync.OpMedia, BrandID: brandID},
		{Type: socialsync.OpAnalytics, BrandID: brandID, Range: "28d"}, // dashboard default
	}
	for _, msg := range jobs {
		queued, err := socialsync.Enqueue(msg)
		if err != nil {
			log.Printf("social_connect: warm-up enqueue %s failed for %s: %v", msg.Type, brandID, err)
			continue
		}
		// queued=false means SOCIAL_SYNC_QUEUE_URL is unset for this lambda, so the
		// job was silently dropped (no inline fallback here — we must not block the
		// OAuth redirect on slow Meta work). Surface it: in prod the connect lambda
		// is expected to have the queue URL, so this points at a misconfiguration.
		if !queued {
			log.Printf("social_connect: warm-up %s for %s NOT queued (SOCIAL_SYNC_QUEUE_URL unset) — inbox/media will only sync on first page open", msg.Type, brandID)
		}
	}
}
