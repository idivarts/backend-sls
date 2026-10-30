package trendlymodels

import (
	"context"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"google.golang.org/api/iterator"
)

// MediaModel is one row of the generative-media capability registry.
//
// It exists because "which models can we use, and what can each one do" is the
// fastest-moving fact in this product — models appear and retire on weeks of
// notice — and encoding it in Go means a deploy for every change. As a
// Firestore collection, enabling a newly published model is a field edit.
//
// The capability flags are not decoration. A video model that cannot take an
// end frame must not be offered an end-frame slot, and a 30-second request to
// a model capped at 8 must be refused rather than silently clamped: the user
// pays for what comes back either way, so a wrong answer costs real money.
//
// ⚠️ EstUSDPerSecond is PROVISIONAL for every row. Per-model credit cost is not
// published by the providers, so these are estimates until the empirical
// measurement pass. Anything user-facing that quotes a price must say so.
type MediaModel struct {
	ID string `json:"id" firestore:"-"`

	// Provider is the mediagen driver that serves it ("openrouter", "pexels").
	Provider    string `json:"provider" firestore:"provider"`
	DisplayName string `json:"displayName" firestore:"displayName"`
	// Kind mirrors mediagen.Kind: image | video | music | voice | stock.
	Kind string `json:"kind" firestore:"kind"`

	// Capabilities. Default false, which is the safe answer: a model we have
	// not described cannot be offered a feature it may not have.
	StartFrame  bool `json:"startFrame,omitempty" firestore:"startFrame"`
	EndFrame    bool `json:"endFrame,omitempty" firestore:"endFrame"`
	ImageRefs   bool `json:"imageRefs,omitempty" firestore:"imageRefs"`
	VideoRefs   bool `json:"videoRefs,omitempty" firestore:"videoRefs"`
	SourceVideo bool `json:"sourceVideo,omitempty" firestore:"sourceVideo"`
	NativeAudio bool `json:"nativeAudio,omitempty" firestore:"nativeAudio"`

	MinDurationS int `json:"minDurationS,omitempty" firestore:"minDurationS"`
	MaxDurationS int `json:"maxDurationS,omitempty" firestore:"maxDurationS"`
	// DurationsS is the EXACT set of lengths a model accepts, when it accepts a
	// set rather than a range.
	//
	// Veo takes 4, 6 or 8 seconds and refuses 5 with a 422 — a bound of 4-8
	// cannot express that, so a composer offering "5s" from the range offered a
	// value the provider would always reject. Empty means the min/max range is
	// the truth, which is the ordinary case.
	DurationsS   []int    `json:"durationsS,omitempty" firestore:"durationsS"`
	Resolutions  []string `json:"resolutions,omitempty" firestore:"resolutions"`
	AspectRatios []string `json:"aspectRatios,omitempty" firestore:"aspectRatios"`

	// EstUSDPerSecond prices video; EstUSDPerCall prices a one-shot image or
	// search. PROVISIONAL — see the type comment.
	EstUSDPerSecond float64 `json:"estUsdPerSecond,omitempty" firestore:"estUsdPerSecond"`
	EstUSDPerCall   float64 `json:"estUsdPerCall,omitempty" firestore:"estUsdPerCall"`

	// MinPlan gates the model. Mirrors openrouter.Plan values.
	MinPlan string `json:"minPlan,omitempty" firestore:"minPlan"`

	// Enabled is the review gate. The catalog sync inserts new models with
	// Enabled=false and NEVER flips it: an untested model that spends money
	// appearing in production is an unreviewed change, however convenient.
	Enabled bool `json:"enabled" firestore:"enabled"`

	CreatedAt int64 `json:"createdAt,omitempty" firestore:"createdAt"`
	UpdatedAt int64 `json:"updatedAt,omitempty" firestore:"updatedAt"`
}

const mediaModelsCollection = "mediaModels"

func mediaModels() *firestore.CollectionRef {
	return firestoredb.Client.Collection(mediaModelsCollection)
}

// ListMediaModels returns every row, enabled or not. Callers that are choosing
// a model want ListEnabledMediaModels instead.
func ListMediaModels(ctx context.Context) ([]MediaModel, error) {
	it := mediaModels().Documents(ctx)
	defer it.Stop()
	var out []MediaModel
	for {
		doc, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var m MediaModel
		if err := doc.DataTo(&m); err != nil {
			continue
		}
		m.ID = doc.Ref.ID
		out = append(out, m)
	}
	return out, nil
}

// ListEnabledMediaModels returns the reviewed models for a kind.
//
// Filters in memory rather than in the query. The collection is a catalog of
// tens of rows read behind a cache, so a composite index on (kind, enabled)
// would buy nothing and is one more thing to keep in step across two index
// files and two database editions.
func ListEnabledMediaModels(ctx context.Context, kind string) ([]MediaModel, error) {
	all, err := ListMediaModels(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]MediaModel, 0, len(all))
	for _, m := range all {
		if m.Enabled && (kind == "" || m.Kind == kind) {
			out = append(out, m)
		}
	}
	return out, nil
}

// GetMediaModel reads one row by id.
func GetMediaModel(ctx context.Context, id string) (*MediaModel, error) {
	doc, err := mediaModels().Doc(id).Get(ctx)
	if err != nil {
		return nil, err
	}
	var m MediaModel
	if err := doc.DataTo(&m); err != nil {
		return nil, err
	}
	m.ID = doc.Ref.ID
	return &m, nil
}

// UpsertMediaModel writes a row, creating it if absent.
//
// Used by the seed script and by the catalog sync. The sync passes
// Enabled=false for anything new and must not overwrite a row an operator has
// already reviewed — see UpsertNewMediaModel.
func UpsertMediaModel(ctx context.Context, m *MediaModel) error {
	now := time.Now().UnixMilli()
	if m.CreatedAt == 0 {
		m.CreatedAt = now
	}
	m.UpdatedAt = now
	_, err := mediaModels().Doc(m.ID).Set(ctx, m)
	return err
}

// UpsertNewMediaModel writes a row ONLY if it does not already exist, and
// reports whether it wrote.
//
// This is the catalog sync's entry point. A plain upsert there would reset
// `enabled` on every poll, quietly disabling models an operator had turned on —
// or, worse if the default ever flipped, enabling ones they had turned off.
func UpsertNewMediaModel(ctx context.Context, m *MediaModel) (bool, error) {
	if _, err := mediaModels().Doc(m.ID).Get(ctx); err == nil {
		return false, nil
	}
	m.Enabled = false
	if err := UpsertMediaModel(ctx, m); err != nil {
		return false, err
	}
	return true, nil
}

// SetMediaModelEnabled flips the review gate on one row.
func SetMediaModelEnabled(ctx context.Context, id string, enabled bool) error {
	_, err := mediaModels().Doc(id).Update(ctx, []firestore.Update{
		{Path: "enabled", Value: enabled},
		{Path: "updatedAt", Value: time.Now().UnixMilli()},
	})
	return err
}

// ---------------------------------------------------------------------------
// Cache
// ---------------------------------------------------------------------------

// The catalog is small, changes rarely, and is read on a path where a Firestore
// round trip per request is pure latency. Cached for a minute so enabling a
// model takes effect without a deploy and without a restart.
var (
	mmMu     sync.RWMutex
	mmCache  []MediaModel
	mmLoaded time.Time
)

const mediaModelTTL = time.Minute

// CachedMediaModels returns the registry, refreshing at most once a minute.
//
// On a read failure it returns the LAST GOOD list rather than an empty one. An
// empty registry reads as "no models exist", which would take every generative
// surface down over a transient Firestore blip.
func CachedMediaModels(ctx context.Context) []MediaModel {
	mmMu.RLock()
	fresh := time.Since(mmLoaded) < mediaModelTTL && mmCache != nil
	cached := mmCache
	mmMu.RUnlock()
	if fresh {
		return cached
	}
	list, err := ListMediaModels(ctx)
	if err != nil {
		return cached
	}
	mmMu.Lock()
	mmCache, mmLoaded = list, time.Now()
	mmMu.Unlock()
	return list
}

// InvalidateMediaModels drops the cache, so a write is visible immediately to
// the process that made it.
func InvalidateMediaModels() {
	mmMu.Lock()
	mmCache, mmLoaded = nil, time.Time{}
	mmMu.Unlock()
}
