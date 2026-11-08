package trendlymodels

// The per-brand storage rollup.
//
// Kept CURRENT rather than computed on demand, because the honest alternative
// is walking every asset row (or worse, the bucket) every time anyone wants the
// number — and the number is wanted on a settings screen, which means on every
// visit.
//
// It is maintained from Phase 0 even though storage is not charged for yet. The
// reason is not tidiness: a counter started later can only ever report "bytes
// since we started counting", and the first thing anyone asks of a storage
// figure is what it was last month. Starting it at the same moment the registry
// starts means the figure is true from the first byte.
//
// ⚠️ ARCHIVING DOES NOT REDUCE THE BYTE TOTAL. Archived media still occupies
// the bucket; only its visible count drops. A storage number that falls when a
// user hides something would under-report what is actually stored, which is the
// one direction a billable figure must never be wrong in.

import (
	"context"
	"log"
	"time"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// BrandMediaUsage is the rollup document at
// brands/{brandId}/aggregates/mediaUsage.
type BrandMediaUsage struct {
	// BytesTotal is everything this brand has ever put in the bucket and not
	// had removed — archived media included.
	BytesTotal int64 `json:"bytesTotal" firestore:"bytesTotal"`
	// CountVisible is the pickable rows: ready, not archived.
	CountVisible int64 `json:"countVisible" firestore:"countVisible"`
	// BytesByKind splits the total by image/video/audio, which is the only
	// breakdown anyone has ever asked for — video is the whole bill.
	BytesByKind map[string]int64 `json:"bytesByKind,omitempty" firestore:"bytesByKind,omitempty"`
	UpdatedAt   int64            `json:"updatedAt,omitempty" firestore:"updatedAt,omitempty"`
}

func mediaUsageDoc(brandID string) *firestore.DocumentRef {
	return firestoredb.Client.Collection("brands").Doc(brandID).
		Collection("aggregates").Doc("mediaUsage")
}

// firestoreCollectionGroupAssets is the collection-group handle for assets.
//
// Wrapped so the sweeps that need it do not each reach for the client; the
// standing rule is that firestoredb.Client appears only in this package, and a
// helper is how that stays true without every caller copying the string.
func firestoreCollectionGroupAssets() firestore.Query {
	return firestoredb.Client.CollectionGroup("assets").Query
}

// bumpMediaUsage moves the rollup by a delta.
//
// Atomic increments, and deliberately BEST EFFORT: a failed rollup write must
// not fail the registration that triggered it. Losing a counter update costs an
// approximate number on a settings screen; losing the registration costs an
// orphaned object in the bucket that nothing knows about, which is the exact
// failure this whole phase exists to prevent.
func bumpMediaUsage(ctx context.Context, brandID, kind string, deltaBytes int64, deltaCount int) {
	if brandID == "" || (deltaBytes == 0 && deltaCount == 0) {
		return
	}
	payload := map[string]interface{}{
		"updatedAt": time.Now().UnixMilli(),
	}
	if deltaBytes != 0 {
		payload["bytesTotal"] = firestore.Increment(deltaBytes)
		if kind != "" {
			payload["bytesByKind"] = map[string]interface{}{
				kind: firestore.Increment(deltaBytes),
			}
		}
	}
	if deltaCount != 0 {
		payload["countVisible"] = firestore.Increment(deltaCount)
	}
	if _, err := mediaUsageDoc(brandID).Set(ctx, payload, firestore.MergeAll); err != nil {
		log.Printf("trendlymodels: media usage rollup for %s: %v", brandID, err)
	}
}

// GetBrandMediaUsage reads the rollup. A brand that has stored nothing has no
// document, and an empty rollup is the right answer for it.
func GetBrandMediaUsage(ctx context.Context, brandID string) (*BrandMediaUsage, error) {
	doc, err := mediaUsageDoc(brandID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return &BrandMediaUsage{}, nil
		}
		return nil, err
	}
	var u BrandMediaUsage
	if err := doc.DataTo(&u); err != nil {
		return nil, err
	}
	return &u, nil
}

// RecomputeBrandMediaUsage rebuilds the rollup from the rows.
//
// The repair tool for the best-effort writes above, and the backfill for brands
// whose media predates the counter. Reads every stored row for the brand, so it
// is a maintenance call and not something a screen triggers.
func RecomputeBrandMediaUsage(ctx context.Context, brandID string) (*BrandMediaUsage, error) {
	rows, err := ListStoredAssets(ctx, brandID, "", 5000)
	if err != nil {
		return nil, err
	}
	out := BrandMediaUsage{BytesByKind: map[string]int64{}, UpdatedAt: time.Now().UnixMilli()}
	for _, a := range rows {
		if a.Status == AssetStatusPending || a.Status == AssetStatusFailed {
			continue
		}
		out.BytesTotal += a.SizeBytes
		if a.Kind != "" {
			out.BytesByKind[a.Kind] += a.SizeBytes
		}
		if assetIsPickable(a) {
			out.CountVisible++
		}
	}
	// A plain Set, not a merge: the point of a recompute is to replace a
	// drifted counter, and merging increments into it would keep the drift.
	if _, err := mediaUsageDoc(brandID).Set(ctx, out); err != nil {
		return nil, err
	}
	return &out, nil
}
