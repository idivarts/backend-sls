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

// Authoring lifecycle values, mirrored in the app.
const (
	// DesignPreviewFailureCheck marks a preparation that failed Hyperframes'
	// own gate — a defect in the film, which a rewrite can fix.
	DesignPreviewFailureCheck = "check"

	DesignAuthorQueued  = "queued"
	DesignAuthorWriting = "writing"
	DesignAuthorDone    = "done"
	DesignAuthorFailed  = "failed"
)

// Preview lifecycle values. The same four words as the other two lifecycles,
// so the app's status rules read the same way whichever is in flight.
const (
	DesignPreviewQueued  = "queued"
	DesignPreviewRunning = "running"
	DesignPreviewDone    = "done"
	DesignPreviewFailed  = "failed"
)

// DesignScene is one clip of the film: a mounted composition with a place on
// the timeline. Mirrors hfproject.Scene, which parses it.
type DesignScene struct {
	ID         string `json:"id" firestore:"id"`
	Src        string `json:"src" firestore:"src"`
	StartMs    int    `json:"startMs" firestore:"startMs"`
	DurationMs int    `json:"durationMs" firestore:"durationMs"`
	TrackIndex int    `json:"trackIndex" firestore:"trackIndex"`
	TrackKind  string `json:"trackKind,omitempty" firestore:"trackKind,omitempty"`
}

// PreviewFrame is one still from the film, with where in it the frame is.
//
// TimeMs and not seconds: every other time on this document is milliseconds,
// and a filmstrip in a mixed unit sorts into the wrong order.
type PreviewFrame struct {
	URL    string `json:"url" firestore:"url"`
	TimeMs int    `json:"timeMs" firestore:"timeMs"`
}

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
	//
	// It is also the REVISION POINTER: it names one of the entries in
	// Revisions, and moving it is what a restore does. See
	// design_revision.go.
	ProjectPrefix string `json:"projectPrefix,omitempty" firestore:"projectPrefix,omitempty"`

	// Revisions is the way back — every state this design's project has
	// been in, newest last, capped at MaxDesignRevisions.
	//
	// On the document rather than in a subcollection because the app
	// already subscribes to the document to draw the studio, so the list
	// arrives with everything else and needs no second read.
	Revisions []DesignRevision `json:"revisions,omitempty" firestore:"revisions,omitempty"`

	// RevisionCounter is how many revisions have ever been allocated. Not
	// len(Revisions), which is pruned — see AllocateDesignRevision for why
	// reusing a number would be worse than leaving a gap. Internal: the app
	// has no use for it.
	RevisionCounter int `json:"-" firestore:"revisionCounter,omitempty"`

	// Variables are the project's declared variables and their current values,
	// keyed by the id in data-composition-variables. This is the only part of
	// a design a phone is expected to edit, which is why it is a flat map here
	// rather than buried in the project files.
	Variables map[string]any `json:"variables,omitempty" firestore:"variables,omitempty"`

	// Authoring state, written by the async design_author job.
	//
	// Separate from the render lifecycle because they are different things
	// happening at different times: a design can be re-authored while its
	// last render is still the current file, and a user watching "writing"
	// needs to know which of the two is in flight.
	AuthorStatus string `json:"authorStatus,omitempty" firestore:"authorStatus,omitempty"`
	AuthorError  string `json:"authorError,omitempty" firestore:"authorError,omitempty"`

	// AuthorAttempt counts how many times this design has been written in the
	// current job, including the first.
	//
	// It exists because the job rewrites a film the gate refused, and a user
	// watching "writing" for the third time deserves to know that it is being
	// FIXED rather than that the first attempt is slow. It is also the only
	// record that an org's tokens went on more than one call.
	AuthorAttempt int `json:"authorAttempt,omitempty" firestore:"authorAttempt,omitempty"`

	// Scenes are the clips the root composition mounts, in timeline order,
	// as parsed from index.html when the project was written.
	//
	// Stored so the APP can draw a timeline and a layer tree. The project
	// itself is HTML in S3; without this the app would have to fetch and
	// parse several documents to know what the film is made of, and would
	// become a second implementation of that parsing — one free to disagree
	// with the renderer's.
	Scenes []DesignScene `json:"scenes,omitempty" firestore:"scenes,omitempty"`

	// PreviewURL is a page that PLAYS this design live, before any render:
	// Hyperframes' own player over the project's own HTML. Written when the
	// project is written, because it is a function of the project.
	PreviewURL string `json:"previewUrl,omitempty" firestore:"previewUrl,omitempty"`

	// Brief is what the user asked for, in their own words, kept so the app
	// can show it back to them.
	//
	// Without it, reopening a design offers "Rewrite with AI" over an empty
	// box: the user cannot see what this film was asked to be, and a rewrite
	// starts from nothing while looking like it starts from something. The
	// brief is also the only record of the user's intent once the model's
	// output has replaced it, which is what makes a revision possible at all.
	Brief string `json:"brief,omitempty" firestore:"brief,omitempty"`

	// Preview state: still frames across the film, so the user can SEE their
	// design before paying to render it.
	//
	// Separate from the render lifecycle on purpose. A preview is cheap and
	// repeatable, a render is neither, and the whole point of this pair is
	// that one comes before the other: Hyperframes' own loop snapshots at
	// step 3 and renders at step 5, because "passing checks alone is not a
	// reason to render". Without this the first time anyone sees their film is
	// after the tokens are gone.
	PreviewStatus   string  `json:"previewStatus,omitempty" firestore:"previewStatus,omitempty"`
	PreviewProgress float64 `json:"previewProgress,omitempty" firestore:"previewProgress,omitempty"`
	PreviewError    string  `json:"previewError,omitempty" firestore:"previewError,omitempty"`

	// PreviewFailure says what KIND of failure PreviewError describes, and
	// PreviewFindings carries every finding behind it.
	//
	// Written by the worker, read by the authoring job. PreviewError is a
	// sentence for a person; this pair is the part a program can act on. Only
	// DesignPreviewFailureCheck means the gate measured a defect in the film
	// itself — anything else is infrastructure, and rewriting the film would
	// spend the org's tokens on a fault the film did not cause.
	PreviewFailure  string         `json:"previewFailure,omitempty" firestore:"previewFailure,omitempty"`
	PreviewFindings []string       `json:"previewFindings,omitempty" firestore:"previewFindings,omitempty"`
	PreviewFrames   []PreviewFrame `json:"previewFrames,omitempty" firestore:"previewFrames,omitempty"`
	PreviewedAt     int64          `json:"previewedAt,omitempty" firestore:"previewedAt,omitempty"`

	// Score is the judged quality of the film, as the shared rubric produces
	// it. Written by the scorer, read by the app; opaque here on purpose, so
	// this package does not depend on the AI package that owns the shape.
	Score    map[string]any `json:"score,omitempty" firestore:"score,omitempty"`
	ScoredAt int64          `json:"scoredAt,omitempty" firestore:"scoredAt,omitempty"`

	// Render state. Progress and failure live on the document rather than in
	// the request, so a render survives a page reload or a locked phone — the
	// worker keeps going and the app re-reads this.
	RenderStatus   string  `json:"renderStatus,omitempty" firestore:"renderStatus,omitempty"`
	RenderProgress float64 `json:"renderProgress,omitempty" firestore:"renderProgress,omitempty"`
	RenderError    string  `json:"renderError,omitempty" firestore:"renderError,omitempty"`

	// ChargedTokens is what the wallet paid for the render currently recorded
	// here, and it is the ONLY record of that debt.
	//
	// The charge happens in the API before the work is queued, so the wallet
	// cannot be raced. The failure happens in the worker, minutes later, in a
	// TypeScript process with no access to the Go wallet code. Without this
	// field there is nothing linking the two: a render that dies in the worker
	// has taken the user's tokens and left no trace of how many. That is not
	// hypothetical — a GSAP bug of ours burned 1981 tokens on dev and produced
	// nothing.
	//
	// It is cleared by ClaimBrandDesignRefund when the debt is repaid, so a
	// non-zero value here means "owed", not "was once charged".
	ChargedTokens int64  `json:"chargedTokens,omitempty" firestore:"chargedTokens,omitempty"`
	RenderURL     string `json:"renderUrl,omitempty" firestore:"renderUrl,omitempty"`
	PosterURL     string `json:"posterUrl,omitempty" firestore:"posterUrl,omitempty"`
	RenderedAt    int64  `json:"renderedAt,omitempty" firestore:"renderedAt,omitempty"`

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

// SetBrandDesignChargedTokens records what the wallet just paid for this
// render, so a failure in the worker can be repaid later.
//
// Bypasses UpdateBrandDesign for the same reason the progress writer does:
// this is render bookkeeping, not an edit, and it must not reorder the list
// someone is reading.
func SetBrandDesignChargedTokens(ctx context.Context, brandID, designID string, tokens int64) error {
	if brandID == "" || designID == "" {
		return fmt.Errorf("SetBrandDesignChargedTokens: empty brandID or designID")
	}
	_, err := designDoc(brandID, designID).Update(ctx, []firestore.Update{
		{Path: "chargedTokens", Value: tokens},
	})
	if err != nil {
		return fmt.Errorf("SetBrandDesignChargedTokens: %w", err)
	}
	return nil
}

// ClaimBrandDesignRefund takes ownership of a failed render's unpaid debt,
// returning how many tokens the caller must now give back.
//
// ⚠️ THE CLEAR AND THE READ ARE ONE TRANSACTION, and that is the whole point.
// The obvious version — read chargedTokens, refund, then clear — double-pays
// whenever two retries overlap, which is exactly what an impatient user does
// to a design that just failed. Here the transaction hands the debt to ONE
// caller: the second gets 0 and refunds nothing.
//
// So a caller must refund precisely what it is handed. Dropping the value on
// the floor loses the user their tokens for good, because the record is gone.
//
// Returns 0 when the design is missing, has no debt, or is not in a failed
// state — a render still in flight has not failed yet, and refunding it would
// pay for work that is about to succeed.
func ClaimBrandDesignRefund(ctx context.Context, brandID, designID string) (int64, error) {
	if brandID == "" || designID == "" {
		return 0, nil
	}
	ref := designDoc(brandID, designID)
	var owed int64

	err := firestoredb.Client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		owed = 0
		snap, err := tx.Get(ref)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return nil
			}
			return err
		}
		var d Design
		if err := snap.DataTo(&d); err != nil {
			return err
		}
		if d.ChargedTokens <= 0 || d.RenderStatus != DesignRenderFailed {
			return nil
		}
		owed = d.ChargedTokens
		return tx.Update(ref, []firestore.Update{{Path: "chargedTokens", Value: firestore.Delete}})
	})
	if err != nil {
		return 0, fmt.Errorf("ClaimBrandDesignRefund: %w", err)
	}
	return owed, nil
}

// MarkBrandDesignPreviewQueued starts a preview run.
//
// The old frames are cleared with it. A filmstrip left over from the previous
// version of the design, sitting next to a spinner, is the user approving
// something that is no longer what would render.
func MarkBrandDesignPreviewQueued(ctx context.Context, brandID, designID string) error {
	return UpdateBrandDesign(ctx, brandID, designID, []firestore.Update{
		{Path: "previewStatus", Value: DesignPreviewQueued},
		{Path: "previewProgress", Value: 0},
		{Path: "previewError", Value: firestore.Delete},
		{Path: "previewFrames", Value: firestore.Delete},
		// With the error, not separately. A run that starts while the last
		// run's findings are still on the document is one the authoring job
		// could read as a fresh gate failure.
		{Path: "previewFailure", Value: firestore.Delete},
		{Path: "previewFindings", Value: firestore.Delete},
	})
}

// SetBrandDesignPreviewProgress is the worker ticking as frames are captured.
// Like the render's, it bypasses UpdateBrandDesign so a preview in flight does
// not keep rewriting updatedAt and reordering the list someone is reading.
func SetBrandDesignPreviewProgress(ctx context.Context, brandID, designID string, fraction float64) error {
	if brandID == "" || designID == "" {
		return fmt.Errorf("SetBrandDesignPreviewProgress: empty brandID or designID")
	}
	if fraction < 0 {
		fraction = 0
	}
	if fraction > 1 {
		fraction = 1
	}
	_, err := designDoc(brandID, designID).Update(ctx, []firestore.Update{
		{Path: "previewStatus", Value: DesignPreviewRunning},
		{Path: "previewProgress", Value: fraction},
	})
	if err != nil {
		return fmt.Errorf("SetBrandDesignPreviewProgress: %w", err)
	}
	return nil
}

// SetBrandDesignPreviewFrames records a finished filmstrip.
func SetBrandDesignPreviewFrames(ctx context.Context, brandID, designID string, frames []PreviewFrame) error {
	if len(frames) == 0 {
		// An empty filmstrip marked done would read as "your design is blank".
		return MarkBrandDesignPreviewFailed(ctx, brandID, designID,
			"We could not capture any frames from this design.")
	}
	return UpdateBrandDesign(ctx, brandID, designID, []firestore.Update{
		{Path: "previewStatus", Value: DesignPreviewDone},
		{Path: "previewProgress", Value: 1},
		{Path: "previewFrames", Value: frames},
		{Path: "previewError", Value: firestore.Delete},
		{Path: "previewFailure", Value: firestore.Delete},
		{Path: "previewFindings", Value: firestore.Delete},
		{Path: "previewedAt", Value: nowMs()},
	})
}

// MarkBrandDesignPreviewFailed records a failure with a sentence the user reads.
func MarkBrandDesignPreviewFailed(ctx context.Context, brandID, designID, reason string) error {
	if reason == "" {
		reason = "We could not preview this design. Please try again."
	}
	return UpdateBrandDesign(ctx, brandID, designID, []firestore.Update{
		{Path: "previewStatus", Value: DesignPreviewFailed},
		{Path: "previewError", Value: reason},
		// This path is only ever reached for OUR failures — a dispatch that
		// did not land, a filmstrip with no frames. None of them is a defect
		// in the film, so the gate marks come off: leaving them would have
		// the authoring job rewrite a film that was never refused.
		{Path: "previewFailure", Value: firestore.Delete},
		{Path: "previewFindings", Value: firestore.Delete},
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

// MarkBrandDesignAuthoring moves a design into the authoring lifecycle and
// clears the previous attempt's error, so a retry does not show the old
// failure beside a job that is working.
func MarkBrandDesignAuthoring(ctx context.Context, brandID, designID, status string) error {
	return UpdateBrandDesign(ctx, brandID, designID, []firestore.Update{
		{Path: "authorStatus", Value: status},
		{Path: "authorError", Value: firestore.Delete},
		// Reset here rather than left to the loop. This is where a run
		// BEGINS, so a design re-queued after an earlier run rewrote it
		// twice would otherwise still read "attempt 3" on its first pass.
		{Path: "authorAttempt", Value: 1},
	})
}

// MarkBrandDesignRewriting puts a design back into authoring to fix what the
// gate refused.
//
// One write, because the three facts belong together. The status says a job is
// working, the attempt number says it is a REWRITE rather than a slow first
// pass, and the preview fields have to go: a design showing "writing" beside
// the last attempt's failure reads as two contradictory things at once, and
// the findings it is being rewritten FROM would otherwise still be sitting
// there when the next run reads them.
//
// authorError is cleared with them. A rewrite that follows a failed attempt
// must not show the old failure next to a job that is in flight.
func MarkBrandDesignRewriting(ctx context.Context, brandID, designID string, attempt int) error {
	if attempt < 1 {
		return fmt.Errorf("MarkBrandDesignRewriting: attempt must be at least 1, got %d", attempt)
	}
	return UpdateBrandDesign(ctx, brandID, designID, []firestore.Update{
		{Path: "authorStatus", Value: DesignAuthorWriting},
		{Path: "authorError", Value: firestore.Delete},
		{Path: "authorAttempt", Value: attempt},
		{Path: "previewStatus", Value: firestore.Delete},
		{Path: "previewProgress", Value: firestore.Delete},
		{Path: "previewError", Value: firestore.Delete},
		{Path: "previewFailure", Value: firestore.Delete},
		{Path: "previewFindings", Value: firestore.Delete},
	})
}

// QueueBrandDesignAuthoring marks a design as queued AND records the brief that
// queued it, in one write.
//
// One write rather than two because the brief and the status it belongs to must
// not be separable: a queued design whose brief failed to save would be written
// from a brief the user can no longer see, and a saved brief with no queued
// status would look like a design that was never asked for.
func QueueBrandDesignAuthoring(ctx context.Context, brandID, designID, brief string) error {
	return UpdateBrandDesign(ctx, brandID, designID, []firestore.Update{
		{Path: "authorStatus", Value: DesignAuthorQueued},
		{Path: "authorError", Value: firestore.Delete},
		{Path: "brief", Value: brief},
	})
}

// MarkBrandDesignAuthorFailed records a failure with a reason the app can
// show. `reason` is read by the user verbatim, so it must be a sentence.
func MarkBrandDesignAuthorFailed(ctx context.Context, brandID, designID, reason string) error {
	if reason == "" {
		reason = "Writing this design did not work. Please try again."
	}
	return UpdateBrandDesign(ctx, brandID, designID, []firestore.Update{
		{Path: "authorStatus", Value: DesignAuthorFailed},
		{Path: "authorError", Value: reason},
	})
}

// SetBrandDesignProject records a freshly authored project.
//
// Both fields together: a projectPrefix without a durationMs is a renderable
// design the wallet cannot price, and a durationMs without the prefix prices
// a design with nothing to render.
func SetBrandDesignProject(
	ctx context.Context,
	brandID, designID, prefix string,
	durationMs int,
	scenes []DesignScene,
	previewURL string,
) error {
	if scenes == nil {
		// An explicit empty list, not a missing field: a rewrite that
		// produced no scenes must CLEAR the previous project's, or the
		// timeline keeps drawing clips from a film that no longer exists.
		scenes = []DesignScene{}
	}
	// An empty previewUrl CLEARS the old one: a rewrite whose player could
	// not be written must not leave the app playing the previous film.
	var preview any = previewURL
	if previewURL == "" {
		preview = firestore.Delete
	}
	return UpdateBrandDesign(ctx, brandID, designID, []firestore.Update{
		{Path: "projectPrefix", Value: prefix},
		{Path: "durationMs", Value: durationMs},
		{Path: "scenes", Value: scenes},
		{Path: "previewUrl", Value: preview},
		{Path: "authorStatus", Value: DesignAuthorDone},
		{Path: "authorError", Value: firestore.Delete},
	})
}

// SetBrandDesignScore stores the judged quality of a design.
//
// Takes an already-encoded map rather than a struct: Firestore ignores `json:`
// tags and would otherwise store Go field names, and the score's own
// FirestoreMap is what produces the shape the app reads. Keeping the encoding
// at the caller also stops this package having to import the AI package, which
// imports this one.
func SetBrandDesignScore(ctx context.Context, brandID, designID string, doc map[string]any) error {
	if brandID == "" || designID == "" {
		return fmt.Errorf("SetBrandDesignScore: empty brandID or designID")
	}
	// Direct, like the progress writers: a score is a judgement ABOUT the
	// design, not an edit of it, and bumping updatedAt would reorder the list
	// someone is reading.
	_, err := designDoc(brandID, designID).Update(ctx, []firestore.Update{
		{Path: "score", Value: doc},
		{Path: "scoredAt", Value: nowMs()},
	})
	if err != nil {
		return fmt.Errorf("SetBrandDesignScore: %w", err)
	}
	return nil
}
