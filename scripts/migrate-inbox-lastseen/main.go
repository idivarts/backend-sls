// One-time migration: baseline lastSeenAt on existing inbox conversations.
//
// Before the unread-badge feature, inbox conversations had no lastSeenAt field.
// Without a baseline the frontend would count all historical inbound messages as
// unread on rollout. This stamps lastSeenAt = lastActivityAt on every conversation
// that has no baseline yet (lastSeenAt == 0), so existing history starts "read".
//
// Run ONCE at rollout:
//   go run ./scripts/migrate-inbox-lastseen
//
// (Running it later would mark genuinely-unread conversations as read.)
package main

import (
	"log"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"

	_ "github.com/idivarts/backend-sls/pkg/firebase"
)

func main() {
	updated, scanned, err := trendlymodels.BaselineInboxLastSeenAt()
	if err != nil {
		log.Fatalf("baseline failed after updating %d/%d: %v", updated, scanned, err)
	}
	log.Printf("inbox lastSeenAt baseline complete: updated %d of %d conversations", updated, scanned)
}
