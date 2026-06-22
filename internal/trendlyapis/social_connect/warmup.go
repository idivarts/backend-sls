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
		if _, err := socialsync.Enqueue(msg); err != nil {
			log.Printf("social_connect: warm-up enqueue %s failed for %s: %v", msg.Type, brandID, err)
		}
	}
}
