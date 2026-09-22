package trendlymodels

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

// RevenuecatTransactionOwner records which org first activated a given
// RevenueCat original transaction id. Subscriptions are not transferable
// between orgs (see RecordRestoreConflict in organization.go): a later
// purchase/renewal event for the SAME transaction id under a DIFFERENT org is
// a restore-on-another-org conflict, not a legitimate new owner.
//
// Collection: revenuecatTransactionOwners/{originalTransactionId}. Doc id IS
// the transaction id, so ownership lookup is a single point read — no
// secondary index needed. Server-only (webhook writes; frontend never
// reads/writes it — see firestore.rules).
type RevenuecatTransactionOwner struct {
	OrgID     string `json:"orgId" firestore:"orgId"`
	ClaimedAt int64  `json:"claimedAt" firestore:"claimedAt"`
}

const revenuecatTransactionOwnersCollection = "revenuecatTransactionOwners"

// ClaimTransactionOwner registers orgID as the owner of transactionID the
// FIRST time it's seen (create-if-absent, race-safe via Firestore's
// AlreadyExists on a duplicate Create — same idempotency pattern as
// MarkWebhookEventProcessed) and always returns the actual owner, which may be
// a DIFFERENT org than the caller's on a later call. A no-op (returns orgID,
// nil) when either id is empty — not every event type carries a transaction id.
func ClaimTransactionOwner(transactionID, orgID string) (ownerOrgID string, err error) {
	if transactionID == "" || orgID == "" {
		return orgID, nil
	}
	ctx := context.Background()
	ref := firestoredb.Client.Collection(revenuecatTransactionOwnersCollection).Doc(transactionID)

	_, err = ref.Create(ctx, &RevenuecatTransactionOwner{
		OrgID:     orgID,
		ClaimedAt: time.Now().UnixMilli(),
	})
	if err == nil {
		return orgID, nil // first claim — this org is the owner
	}
	if status.Code(err) != codes.AlreadyExists {
		return "", err
	}

	snap, getErr := ref.Get(ctx)
	if getErr != nil {
		return "", getErr
	}
	var existing RevenuecatTransactionOwner
	if err := snap.DataTo(&existing); err != nil {
		return "", err
	}
	return existing.OrgID, nil
}
