package trendlymodels

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

// Strategy mirrors a content-strategy document at
// brands/{brandId}/strategies/{strategyId}. It is authored/edited by the brand
// app and the AI strategy module, then pushed to the content calendar. Only the
// fields the backend reads or writes are modelled; the live web editor stores
// additional CRDT state under the `yupdates` subcollection (pruned via
// PruneStrategyYUpdates after an AI rewrite).

const strategiesSubcollection = "strategies"
const strategyYUpdatesSubcollection = "yupdates"

// StrategyTimeline is the strategy's nominal run window in epoch ms. At creation
// the startDate is nominal (now) — the authoritative placement is chosen at
// push-to-calendar time; only the span (duration) is meaningful here.
type StrategyTimeline struct {
	StartDate int64 `json:"startDate" firestore:"startDate"`
	EndDate   int64 `json:"endDate" firestore:"endDate"`
}

type Strategy struct {
	ID              string            `json:"id,omitempty" firestore:"-"`
	Name            string            `json:"name" firestore:"name"`
	Objective       string            `json:"objective,omitempty" firestore:"objective,omitempty"`
	ManagerID       string            `json:"managerId,omitempty" firestore:"managerId,omitempty"`
	Status          string            `json:"status" firestore:"status"`
	ReviewStatus    string            `json:"reviewStatus,omitempty" firestore:"reviewStatus,omitempty"`
	MarkdownContent string            `json:"markdownContent,omitempty" firestore:"markdownContent,omitempty"`
	Platforms       []string          `json:"platforms,omitempty" firestore:"platforms,omitempty"`
	ContentFormats  []string          `json:"contentFormats,omitempty" firestore:"contentFormats,omitempty"`
	Timeline        *StrategyTimeline `json:"timeline,omitempty" firestore:"timeline,omitempty"`
	CrdtInitialized bool              `json:"crdtInitialized,omitempty" firestore:"crdtInitialized,omitempty"`
	CrdtGeneration  int64             `json:"crdtGeneration,omitempty" firestore:"crdtGeneration,omitempty"`
	CreatedAt       int64             `json:"createdAt,omitempty" firestore:"createdAt,omitempty"`
	UpdatedAt       int64             `json:"updatedAt,omitempty" firestore:"updatedAt,omitempty"`
	LastEditedAt    int64             `json:"lastEditedAt,omitempty" firestore:"lastEditedAt,omitempty"`
}

func strategiesCollection(brandID string) *firestore.CollectionRef {
	return firestoredb.Client.Collection("brands").Doc(brandID).Collection(strategiesSubcollection)
}

func strategyDoc(brandID, strategyID string) *firestore.DocumentRef {
	return strategiesCollection(brandID).Doc(strategyID)
}

// CreateStrategy adds a new strategy document and returns its generated id. The
// caller supplies the full field map (so AI/onboarding can stamp their own seed
// fields); createdAt/updatedAt are filled in only when absent.
func CreateStrategy(ctx context.Context, brandID string, fields map[string]any) (string, error) {
	if brandID == "" {
		return "", fmt.Errorf("CreateStrategy: empty brandID")
	}
	ref, _, err := strategiesCollection(brandID).Add(ctx, fields)
	if err != nil {
		return "", err
	}
	return ref.ID, nil
}

// GetStrategy reads a single strategy document, populating ID from the doc id.
func GetStrategy(ctx context.Context, brandID, strategyID string) (*Strategy, error) {
	doc, err := strategyDoc(brandID, strategyID).Get(ctx)
	if err != nil {
		return nil, err
	}
	var s Strategy
	if err := doc.DataTo(&s); err != nil {
		return nil, err
	}
	s.ID = doc.Ref.ID
	return &s, nil
}

// UpdateStrategy applies a partial update to a strategy document. Callers build
// the []firestore.Update (so they can use firestore.Increment, FieldPath edits,
// etc.) — the Firestore call itself lives here in the model.
func UpdateStrategy(ctx context.Context, brandID, strategyID string, updates []firestore.Update) error {
	_, err := strategyDoc(brandID, strategyID).Update(ctx, updates)
	return err
}

// PruneStrategyYUpdates deletes the Yjs CRDT update log under a strategy. Used
// after an AI rewrite invalidates the CRDT baseline — best-effort storage
// hygiene, no correctness depends on it.
func PruneStrategyYUpdates(ctx context.Context, brandID, strategyID string) {
	iter := strategyDoc(brandID, strategyID).Collection(strategyYUpdatesSubcollection).Documents(ctx)
	defer iter.Stop()
	bw := firestoredb.Client.BulkWriter(ctx)
	for {
		doc, err := iter.Next()
		if err != nil {
			break
		}
		_, _ = bw.Delete(doc.Ref)
	}
	bw.End()
}
