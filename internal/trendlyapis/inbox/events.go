package inbox

import (
	"log"

	"github.com/idivarts/backend-sls/internal/socialsync"
)

// enqueueOrRun hands a brand-wide inbox op to the shared social queue.
func enqueueOrRun(brandID string, op socialsync.OpType) error {
	return enqueueOrRunMsg(socialsync.Message{Type: op, BrandID: brandID}, func() error {
		switch op {
		case socialsync.OpMedia:
			return RefreshMedia(brandID)
		default: // OpInboxSync
			return SyncFromMeta(brandID)
		}
	})
}

// enqueueOrRunMsg sends msg to the shared social queue (see internal/socialsync).
// If no queue is configured it runs `inline` as a fallback — the worker logic
// writes to Firestore either way, so the frontend's listener behaves the same;
// the inline path is just synchronous (used in local dev). `inline` must perform
// the same work the worker would for msg.Type.
func enqueueOrRunMsg(msg socialsync.Message, inline func() error) error {
	queued, err := socialsync.Enqueue(msg)
	if err != nil {
		return err
	}
	if queued {
		return nil
	}
	log.Printf("inbox: %s queue not set — running %s inline for %s", socialsync.QueueEnv, msg.Type, msg.BrandID)
	return inline()
}
