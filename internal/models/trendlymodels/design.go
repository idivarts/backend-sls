package trendlymodels

// A design, as Design Studio v3 understands it.
//
// ── Why this is a top-level collection ──────────────────────────────────────
// v1 and v2 stored a design under the content that owned it:
//
//	brands/{brandId}/contents/{contentId}/studios/{studioId}
//
// The content id was in the PATH, so a design could not exist without a
// content, could not be opened from the drawer, and could not outlive the
// content it was made for. v3 inverts that: a design is its own document and
// the link to a content is an optional FIELD.
//
//	brands/{brandId}/designs/{designId}   + ContentID "" | "<id>"
//
// That one change is what lets Design Studio sit in the Playground section of
// the drawer as a thing you open, rather than a mode you enter from a content.
//
// ⚠️ The delete path is where "independent" is easiest to break. Deleting a
// content must CLEAR ContentID on its designs, never delete them — see
// UnlinkDesignsFromContent. A cascade here would quietly restore the v2
// behaviour while the schema still claimed otherwise.
//
// ── Why the project lives in S3, not here ───────────────────────────────────
// A Hyperframes project is a directory — index.html, compositions/*.html and
// assets/ — not a document. Firestore holds the metadata, the render state and
// the pointer; the files live under ProjectPrefix in the render bucket. The
// worker materialises them into /project and runs the CLI there.

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// NOTE: the exported helpers are named BrandDesign* because content_design.go
// already owns Design* for the v2 content-scoped revisions. Both collections
// are alive until Phase 6 retires v2; the rename back to Design* belongs in
// that change, not before.
// ⚠️ THE COLLECTION ID `designs` IS SHARED WITH v2.
//
// v2 stores content-scoped revisions at
// brands/{brandId}/contents/{contentId}/designs/{revisionId} — the same
// collection id as this one. content_design.go ListStuckRenders runs a
// COLLECTION-GROUP query over `designs`, so it sees both kinds of document.
//
// Today that is harmless, by one detail: the sweep filters on
// `renderStartedAt < cutoff`, a v3 design has no such field, and a Firestore
// inequality skips documents where the field is absent. The path guard in
// that loop would skip them anyway.
//
// So do NOT add a `renderStartedAt` field here without giving v3 its own
// sweep. The moment it exists, v3 designs enter that query, get dropped by
// the path guard, and a stranded v3 render is invisible to the only thing
// that closes stranded renders — while the sweep still looks healthy.
// design_test.go asserts the field's absence so this cannot happen quietly.
const designsSubcollection = "designs"

// The render lifecycle strings are the ones content_design.go already owns
// (DesignRenderQueued / Rendering / Done / Failed), mirrored in shared-libs
// design.ts. They are reused rather than redeclared so v2 and v3 can never
// drift apart while both are alive.

// DesignFormat is the canvas. Hyperframes reads these off the root composition
// as data-width / data-height, and the render is produced at exactly this size
// — Phase 0 confirmed that changing them and re-rendering is all a reframe
// takes, because the scenes lay out in CSS rather than in fixed slots.
type DesignFormat struct {
	Width  int `json:"width" firestore:"width"`
	Height int `json:"height" firestore:"height"`
	FPS    int `json:"fps" firestore:"fps"`
}

type Design struct {
	ID   string `json:"id,omitempty" firestore:"-"`
	Name string `json:"name" firestore:"name"`

	// ContentID links this design to a content, or is empty when it stands on
	// its own. It decides where an export lands: set, the file is attached to
	// that content; empty, the design keeps its own media and is shared by
	// link. Nothing else about the design depends on it.
	ContentID string `json:"contentId,omitempty" firestore:"contentId,omitempty"`

	ManagerID string       `json:"managerId,omitempty" firestore:"managerId,omitempty"`
	Format    DesignFormat `json:"format" firestore:"format"`

	// DurationMs is the film's length, read off the root composition's
	// data-duration when the project is written. It is here rather than
	// derived at render time because the WALLET needs it before the worker
	// ever opens the project — a charge that can only be computed after the
	// render has started is not a gate.
	DurationMs int `json:"durationMs,omitempty" firestore:"durationMs,omitempty"`

	// ProjectPrefix is the S3 key prefix holding the Hyperframes project
	// directory. Empty until the director has written one.
	ProjectPrefix string `json:"projectPrefix,omitempty" firestore:"projectPrefix,omitempty"`

	// Variables are the project's declared variables and their current values,
	// keyed by the id in data-composition-variables. This is the only part of
	// a design a phone is expected to edit, which is why it is a flat map here
	// rather than buried in the project files.
	Variables map[string]any `json:"variables,omitempty" firestore:"variables,omitempty"`

	// Render state. Progress and failure live on the document rather than in
	// the request, so a render survives a page reload or a locked phone — the
	// worker keeps going and the app re-reads this.
	RenderStatus   string  `json:"renderStatus,omitempty" firestore:"renderStatus,omitempty"`
	RenderProgress float64 `json:"renderProgress,omitempty" firestore:"renderProgress,omitempty"`
	RenderError    string  `json:"renderError,omitempty" firestore:"renderError,omitempty"`
	RenderURL      string  `json:"renderUrl,omitempty" firestore:"renderUrl,omitempty"`
	PosterURL      string  `json:"posterUrl,omitempty" firestore:"posterUrl,omitempty"`
	RenderedAt     int64   `json:"renderedAt,omitempty" firestore:"renderedAt,omitempty"`

	CreatedAt int64 `json:"createdAt,omitempty" firestore:"createdAt,omitempty"`
	UpdatedAt int64 `json:"updatedAt,omitempty" firestore:"updatedAt,omitempty"`
}

func designsCollection(brandID string) *firestore.CollectionRef {
	return firestoredb.Client.Collection("brands").Doc(brandID).Collection(designsSubcollection)
}

func designDoc(brandID, designID string) *firestore.DocumentRef {
	return designsCollection(brandID).Doc(designID)
}

func nowMs() int64 { return time.Now().UnixMilli() }

// CreateDesign writes a new design and returns its generated id. contentID may
// be empty: that is a standalone design, not an error.
func CreateBrandDesign(ctx context.Context, brandID string, d *Design) (string, error) {
	if brandID == "" {
		return "", fmt.Errorf("CreateBrandDesign: empty brandID")
	}
	if d == nil {
		return "", fmt.Errorf("CreateBrandDesign: nil design")
	}
	if d.Format.Width <= 0 || d.Format.Height <= 0 {
		return "", fmt.Errorf("CreateBrandDesign: format must have a positive width and height")
	}
	if d.Format.FPS <= 0 {
		d.Format.FPS = 30
	}
	if d.CreatedAt == 0 {
		d.CreatedAt = nowMs()
	}
	d.UpdatedAt = nowMs()

	ref, _, err := designsCollection(brandID).Add(ctx, d)
	if err != nil {
		return "", fmt.Errorf("CreateBrandDesign: %w", err)
	}
	d.ID = ref.ID
	return ref.ID, nil
}

// GetDesign reads one design. A missing document returns a nil Design and a
// nil error, so callers can tell "not there" from "the read failed".
func GetBrandDesign(ctx context.Context, brandID, designID string) (*Design, error) {
	if brandID == "" || designID == "" {
		return nil, fmt.Errorf("GetBrandDesign: empty brandID or designID")
	}
	snap, err := designDoc(brandID, designID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("GetBrandDesign: %w", err)
	}
	var d Design
	if err := snap.DataTo(&d); err != nil {
		return nil, fmt.Errorf("GetBrandDesign: decode: %w", err)
	}
	d.ID = snap.Ref.ID
	return &d, nil
}

// ListDesignsForContent returns the designs linked to one content, newest
// first.
//
// ⚠️ This is the query behind the composite index on (contentId, updatedAt
// desc) in BOTH firestore.indexes.json and firestore.indexes.enterprise.json.
// Indexed in only one of them, it hard-errors on dev or silently full-scans on
// prod Enterprise.
func ListBrandDesignsForContent(ctx context.Context, brandID, contentID string, limit int) ([]Design, error) {
	if brandID == "" || contentID == "" {
		return nil, fmt.Errorf("ListBrandDesignsForContent: empty brandID or contentID")
	}
	if limit <= 0 {
		limit = 50
	}
	snaps, err := designsCollection(brandID).
		Where("contentId", "==", contentID).
		OrderBy("updatedAt", firestore.Desc).
		Limit(limit).
		Documents(ctx).GetAll()
	if err != nil {
		return nil, fmt.Errorf("ListBrandDesignsForContent: %w", err)
	}
	return decodeDesigns(snaps)
}

// ListDesigns returns the brand's designs, newest first — what the Design
// Studio index screen shows, linked or not.
func ListBrandDesigns(ctx context.Context, brandID string, limit int) ([]Design, error) {
	if brandID == "" {
		return nil, fmt.Errorf("ListBrandDesigns: empty brandID")
	}
	if limit <= 0 {
		limit = 50
	}
	snaps, err := designsCollection(brandID).
		OrderBy("updatedAt", firestore.Desc).
		Limit(limit).
		Documents(ctx).GetAll()
	if err != nil {
		return nil, fmt.Errorf("ListBrandDesigns: %w", err)
	}
	return decodeDesigns(snaps)
}

func decodeDesigns(snaps []*firestore.DocumentSnapshot) ([]Design, error) {
	out := make([]Design, 0, len(snaps))
	for _, snap := range snaps {
		var d Design
		if err := snap.DataTo(&d); err != nil {
			return nil, fmt.Errorf("decodeDesigns: %s: %w", snap.Ref.ID, err)
		}
		d.ID = snap.Ref.ID
		out = append(out, d)
	}
	return out, nil
}

// UpdateDesign applies a partial update. Callers pass firestore.Update values
// so a field nobody touched is never rewritten; updatedAt is always stamped.
func UpdateBrandDesign(ctx context.Context, brandID, designID string, updates []firestore.Update) error {
	if brandID == "" || designID == "" {
		return fmt.Errorf("UpdateBrandDesign: empty brandID or designID")
	}
	updates = append(updates, firestore.Update{Path: "updatedAt", Value: nowMs()})
	if _, err := designDoc(brandID, designID).Update(ctx, updates); err != nil {
		return fmt.Errorf("UpdateBrandDesign: %w", err)
	}
	return nil
}

// UnlinkDesignsFromContent clears contentId on every design that pointed at a
// content, and is what a content delete calls INSTEAD of deleting them.
//
// This function is the whole of "a design is independent" as far as the data
// is concerned. If a future content-delete path stops calling it, designs
// start disappearing with their content again and nothing will fail loudly.
func UnlinkBrandDesignsFromContent(ctx context.Context, brandID, contentID string) (int, error) {
	if brandID == "" || contentID == "" {
		return 0, fmt.Errorf("UnlinkBrandDesignsFromContent: empty brandID or contentID")
	}
	snaps, err := designsCollection(brandID).
		Where("contentId", "==", contentID).
		Documents(ctx).GetAll()
	if err != nil {
		return 0, fmt.Errorf("UnlinkBrandDesignsFromContent: %w", err)
	}
	unlinked := 0
	for _, snap := range snaps {
		_, err := snap.Ref.Update(ctx, []firestore.Update{
			{Path: "contentId", Value: firestore.Delete},
			{Path: "updatedAt", Value: nowMs()},
		})
		if err != nil {
			return unlinked, fmt.Errorf("UnlinkBrandDesignsFromContent: %s: %w", snap.Ref.ID, err)
		}
		unlinked++
	}
	return unlinked, nil
}

// MarkDesignRenderQueued moves a design into the render lifecycle and clears
// the previous attempt's error, so a retry does not show the old failure while
// the new one runs.
func MarkBrandDesignRenderQueued(ctx context.Context, brandID, designID string) error {
	return UpdateBrandDesign(ctx, brandID, designID, []firestore.Update{
		{Path: "renderStatus", Value: DesignRenderQueued},
		{Path: "renderProgress", Value: 0},
		{Path: "renderError", Value: firestore.Delete},
	})
}

// SetDesignRenderProgress is called by the worker as frames land.
// ⚠️ This deliberately does NOT go through UpdateBrandDesign, because that
// stamps updatedAt — and updatedAt is the key ListBrandDesigns sorts on. A
// render reports progress many times over, so routing it through the normal
// update would make a rendering design climb to the top of the index list
// again and again while someone is reading it, and rewrite the sort key for
// every live subscriber on each tick. Progress is render state, not an edit.
func SetBrandDesignRenderProgress(ctx context.Context, brandID, designID string, fraction float64) error {
	if brandID == "" || designID == "" {
		return fmt.Errorf("SetBrandDesignRenderProgress: empty brandID or designID")
	}
	if fraction < 0 {
		fraction = 0
	}
	if fraction > 1 {
		fraction = 1
	}
	_, err := designDoc(brandID, designID).Update(ctx, []firestore.Update{
		{Path: "renderStatus", Value: DesignRenderRendering},
		{Path: "renderProgress", Value: fraction},
	})
	if err != nil {
		return fmt.Errorf("SetBrandDesignRenderProgress: %w", err)
	}
	return nil
}

// SetDesignRenderResult records a finished render.
func SetBrandDesignRenderResult(ctx context.Context, brandID, designID, url, posterURL string) error {
	return UpdateBrandDesign(ctx, brandID, designID, []firestore.Update{
		{Path: "renderStatus", Value: DesignRenderDone},
		{Path: "renderProgress", Value: 1},
		{Path: "renderUrl", Value: url},
		{Path: "posterUrl", Value: posterURL},
		{Path: "renderedAt", Value: nowMs()},
		{Path: "renderError", Value: firestore.Delete},
	})
}

// MarkDesignRenderFailed records a failure with a reason the app can show.
func MarkBrandDesignRenderFailed(ctx context.Context, brandID, designID, reason string) error {
	if reason == "" {
		reason = "The render failed."
	}
	return UpdateBrandDesign(ctx, brandID, designID, []firestore.Update{
		{Path: "renderStatus", Value: DesignRenderFailed},
		{Path: "renderError", Value: reason},
	})
}

// DeleteDesign removes a design. The caller is responsible for its S3 project
// prefix; this only touches Firestore.
func DeleteBrandDesign(ctx context.Context, brandID, designID string) error {
	if brandID == "" || designID == "" {
		return fmt.Errorf("DeleteBrandDesign: empty brandID or designID")
	}
	if _, err := designDoc(brandID, designID).Delete(ctx); err != nil {
		return fmt.Errorf("DeleteBrandDesign: %w", err)
	}
	return nil
}
