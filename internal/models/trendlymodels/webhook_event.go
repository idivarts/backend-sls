package trendlymodels

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

// ProcessedWebhookEvent records that an external webhook event id has already
// been handled, so a provider that retries delivery (RevenueCat, Razorpay, …)
// does not apply the same side effect twice. This matters especially for
// additive operations like token top-up credits (AddTopup) where re-processing
// would double-credit the wallet.
//
// Collection: webhookEvents/{provider_eventId}. Server-only (Admin SDK writes;
// the frontend never reads/writes it — see firestore.rules).
type ProcessedWebhookEvent struct {
	Provider    string `json:"provider" firestore:"provider"`
	EventID     string `json:"eventId" firestore:"eventId"`
	ProcessedAt int64  `json:"processedAt" firestore:"processedAt"`
}

const webhookEventsCollection = "webhookEvents"

// MarkWebhookEventProcessed atomically records that (provider, eventID) has been
// handled. It returns isNew=true when this is the first time the event is seen
// (the caller should then apply the event's side effects) and isNew=false when
// the event was already processed (the caller should ack + no-op). Uses a
// create-if-absent write: Firestore returns AlreadyExists on a duplicate, which
// makes this a race-safe idempotency guard.
func MarkWebhookEventProcessed(provider, eventID string) (isNew bool, err error) {
	if eventID == "" {
		// No id to dedupe on — treat as new so the side effect still runs.
		return true, nil
	}
	docID := provider + "_" + eventID
	_, err = firestoredb.Client.Collection(webhookEventsCollection).Doc(docID).
		Create(context.Background(), &ProcessedWebhookEvent{
			Provider:    provider,
			EventID:     eventID,
			ProcessedAt: time.Now().UnixMilli(),
		})
	if err != nil {
		if status.Code(err) == codes.AlreadyExists {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
