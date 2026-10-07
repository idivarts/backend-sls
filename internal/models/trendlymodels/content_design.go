package trendlymodels

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ── HTML design model (supersedes the scene-graph JSON approach) ─────────────
// A design is a single self-contained HTML document (inline CSS + embedded font)
// sized to the exact post dimensions. LLMs author HTML/CSS far more reliably
// than layout JSON, and a real browser (WebView on device) renders it perfectly.
// The app captures the WebView to PNG on approve, so preview == export.
//
// Revisions live at brands/{brandId}/contents/{contentId}/designs/{revisionId}.
// This subcollection is CLIENT-WRITABLE (HTML is not sensitive) so deterministic
// text edits + render-capture happen frontend-side; the AI also writes via the
// Admin SDK (bypasses rules).

// ContentDesignRef is the lightweight pointer cached on the content doc.
type ContentDesignRef struct {
	RevisionID string `json:"revisionId" firestore:"revisionId"`
	DocType    string `json:"docType" firestore:"docType"` // image|video
	Width      int    `json:"width" firestore:"width"`     // per-slide width
	Height     int    `json:"height" firestore:"height"`   // per-slide height
	// SlideCount is the number of carousel slides (1 for a single post).
	SlideCount int `json:"slideCount" firestore:"slideCount"`
	// DurationMs is the animation length for a video design (0 for images).
	DurationMs int `json:"durationMs,omitempty" firestore:"durationMs,omitempty"`
	// RenderURL is the first frontend-captured PNG (cover) used for publish/Canva.
	RenderURL string `json:"renderUrl,omitempty" firestore:"renderUrl,omitempty"`
	UpdatedAt int64  `json:"updatedAt" firestore:"updatedAt"`
}

// ContentDesignRevision is one immutable HTML snapshot.
type ContentDesignRevision struct {
	ID         string `json:"id" firestore:"-"`
	HTML       string `json:"html" firestore:"html"`
	Width      int    `json:"width" firestore:"width"`   // per-slide width
	Height     int    `json:"height" firestore:"height"` // per-slide height
	SlideCount int    `json:"slideCount" firestore:"slideCount"`
	DurationMs int    `json:"durationMs,omitempty" firestore:"durationMs,omitempty"`
	DocType    string `json:"docType" firestore:"docType"` // image|video
	// Origin: "generate" | "edit" | "text" | "revert".
	Origin           string `json:"origin,omitempty" firestore:"origin,omitempty"`
	ParentRevisionID string `json:"parentRevisionId,omitempty" firestore:"parentRevisionId,omitempty"`
	// RenderURL is the cover PNG (images) or the MP4 (video). Written ONLY by the
	// render worker — clients are blocked from it by firestore.rules.
	RenderURL string `json:"renderUrl,omitempty" firestore:"renderUrl,omitempty"`
	// PosterURL is the first frame of a video render, for previews that can't play.
	PosterURL string `json:"posterUrl,omitempty" firestore:"posterUrl,omitempty"`
	// RenderStatus is the server-render lifecycle; empty on pre-worker revisions,
	// where a non-empty RenderURL still means "rendered".
	RenderStatus    string  `json:"renderStatus,omitempty" firestore:"renderStatus,omitempty"`
	RenderProgress  float64 `json:"renderProgress,omitempty" firestore:"renderProgress,omitempty"`
	RenderError     string  `json:"renderError,omitempty" firestore:"renderError,omitempty"`
	RenderJobID     string  `json:"renderJobId,omitempty" firestore:"renderJobId,omitempty"`
	RenderStartedAt int64   `json:"renderStartedAt,omitempty" firestore:"renderStartedAt,omitempty"`
	RenderedAt      int64   `json:"renderedAt,omitempty" firestore:"renderedAt,omitempty"`
	// RenderScale is the deviceScaleFactor used, so a re-render matches exactly.
	RenderScale float64 `json:"renderScale,omitempty" firestore:"renderScale,omitempty"`
	// RenderChargedTokens is what this revision has already cost the wallet.
	// Non-zero means a retry after a failure is FREE — the user paid for the
	// attempt we lost. A forced re-render of a finished revision clears it and
	// charges again, because that is a deliberate new request.
	RenderChargedTokens int64 `json:"renderChargedTokens,omitempty" firestore:"renderChargedTokens,omitempty"`
	CreatedAt           int64 `json:"createdAt" firestore:"createdAt"`
}

// Design render lifecycle values. Mirrored in shared-libs design.ts
// (DesignRenderStatus) — change both together.
const (
	DesignRenderQueued    = "queued"
	DesignRenderRendering = "rendering"
	DesignRenderDone      = "done"
	DesignRenderFailed    = "failed"
)

func contentDesignsCollection(brandID, contentID string) *firestore.CollectionRef {
	return firestoredb.Client.Collection(
		fmt.Sprintf("brands/%s/contents/%s/designs", brandID, contentID))
}

// CreateDesignRevision writes a new immutable HTML design revision.
func CreateDesignRevision(brandID, contentID string, rev *ContentDesignRevision) (string, error) {
	if rev.CreatedAt == 0 {
		rev.CreatedAt = time.Now().UnixMilli()
	}
	ref, _, err := contentDesignsCollection(brandID, contentID).Add(context.Background(), rev)
	if err != nil {
		return "", fmt.Errorf("CreateDesignRevision: %w", err)
	}
	return ref.ID, nil
}

// GetDesignRevision reads one revision, or (nil, nil) if absent.
func GetDesignRevision(brandID, contentID, revisionID string) (*ContentDesignRevision, error) {
	doc, err := contentDesignsCollection(brandID, contentID).Doc(revisionID).Get(context.Background())
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}
	var r ContentDesignRevision
	if err := doc.DataTo(&r); err != nil {
		return nil, err
	}
	r.ID = doc.Ref.ID
	return &r, nil
}

// ListDesignRevisions returns a content's design revisions newest-first.
func ListDesignRevisions(brandID, contentID string, limit int) ([]ContentDesignRevision, error) {
	q := contentDesignsCollection(brandID, contentID).OrderBy("createdAt", firestore.Desc)
	if limit > 0 {
		q = q.Limit(limit)
	}
	iter := q.Documents(context.Background())
	defer iter.Stop()
	out := []ContentDesignRevision{}
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var r ContentDesignRevision
		if err := doc.DataTo(&r); err != nil {
			return nil, err
		}
		r.ID = doc.Ref.ID
		out = append(out, r)
	}
	return out, nil
}

// SetContentDesignRef points the content at a design revision and stamps source.
func SetContentDesignRef(brandID, contentID string, ref *ContentDesignRef) error {
	ref.UpdatedAt = time.Now().UnixMilli()
	return UpdateContentFields(brandID, contentID, map[string]interface{}{
		"designRef": ref,
		"source":    "ai",
	})
}

// updateDesignRevisionFields merges fields into one revision doc. Every render
// write goes through here so the worker never touches firestoredb.Client.
func updateDesignRevisionFields(brandID, contentID, revisionID string, fields map[string]interface{}) error {
	_, err := contentDesignsCollection(brandID, contentID).
		Doc(revisionID).
		Set(context.Background(), fields, firestore.MergeAll)
	if err != nil {
		return fmt.Errorf("updateDesignRevisionFields(%s): %w", revisionID, err)
	}
	return nil
}

// MarkDesignRenderQueued stamps a revision the moment a render is submitted, so
// the UI can show "queued" before any worker has picked the job up. Clears the
// previous error and progress — a retry must not inherit the last failure.
func MarkDesignRenderQueued(brandID, contentID, revisionID, jobID string) error {
	return updateDesignRevisionFields(brandID, contentID, revisionID, map[string]interface{}{
		"renderStatus":    DesignRenderQueued,
		"renderJobId":     jobID,
		"renderError":     firestore.Delete,
		"renderProgress":  firestore.Delete,
		"renderStartedAt": firestore.Delete,
	})
}

// MarkDesignRenderStarted is called by the worker when it picks the job up.
// RenderStartedAt is what the reconciliation cron uses to expire stuck renders.
func MarkDesignRenderStarted(brandID, contentID, revisionID, jobID string) error {
	fields := map[string]interface{}{
		"renderStatus":    DesignRenderRendering,
		"renderStartedAt": time.Now().UnixMilli(),
	}
	if jobID != "" {
		fields["renderJobId"] = jobID
	}
	return updateDesignRevisionFields(brandID, contentID, revisionID, fields)
}

// SetDesignRenderProgress reports 0..1 during a video render. Throttled by the
// caller (roughly every 5% of frames) — this is a Firestore write per call.
func SetDesignRenderProgress(brandID, contentID, revisionID string, progress float64) error {
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	return updateDesignRevisionFields(brandID, contentID, revisionID, map[string]interface{}{
		"renderProgress": progress,
	})
}

// MarkDesignRenderFailed records a user-facing reason. The reason is shown in
// the app verbatim, so it must read as a sentence, not a stack trace.
func MarkDesignRenderFailed(brandID, contentID, revisionID, reason string) error {
	return updateDesignRevisionFields(brandID, contentID, revisionID, map[string]interface{}{
		"renderStatus":   DesignRenderFailed,
		"renderError":    reason,
		"renderProgress": firestore.Delete,
	})
}

// SetDesignRenderResult records a finished render on the revision. This is the
// single field that separates "a design exists" from "a publishable asset
// exists", so it is written last, after the upload has succeeded.
func SetDesignRenderResult(brandID, contentID, revisionID, renderURL, posterURL string, scale float64) error {
	fields := map[string]interface{}{
		"renderStatus":   DesignRenderDone,
		"renderUrl":      renderURL,
		"renderedAt":     time.Now().UnixMilli(),
		"renderError":    firestore.Delete,
		"renderProgress": firestore.Delete,
	}
	if posterURL != "" {
		fields["posterUrl"] = posterURL
	}
	if scale > 0 {
		fields["renderScale"] = scale
	}
	return updateDesignRevisionFields(brandID, contentID, revisionID, fields)
}

// SetContentRenderOutput publishes the render onto the content itself: the
// attachments are what the publish pipeline actually reads, and designRef
// .renderUrl is the cached cover used by share previews and Canva.
//
// Several image attachments are treated as a carousel downstream, so the order
// here is the slide order.
func SetContentRenderOutput(brandID, contentID, renderURL string, attachments []ContentAttachment) error {
	return UpdateContentFields(brandID, contentID, map[string]interface{}{
		"designRef": map[string]interface{}{
			"renderUrl": renderURL,
		},
		"attachments": attachments,
	})
}

// MarkDesignRenderCharged records what the wallet was debited for this revision,
// so a retry after a failed render is not charged a second time.
func MarkDesignRenderCharged(brandID, contentID, revisionID string, tokens int64) error {
	return updateDesignRevisionFields(brandID, contentID, revisionID, map[string]interface{}{
		"renderChargedTokens": tokens,
	})
}

// StuckRender identifies a revision whose render never reported back, with
// enough path context to act on it.
type StuckRender struct {
	BrandID    string
	ContentID  string
	RevisionID string
	StartedAt  int64
}

// ListStuckRenders finds revisions that have been "rendering" since before
// `cutoffMs` — a worker that died, a task killed after its last retry, or a
// message that vanished.
//
// Without this, such a render stays "rendering" forever: the app shows a
// spinner nobody can cancel, and the publish gate blocks on a job that is never
// coming back. The query is a collection-group scan over `designs`, so it needs
// the composite index on (renderStatus, renderStartedAt) that ships in both
// index files.
func ListStuckRenders(ctx context.Context, cutoffMs int64, limit int) ([]StuckRender, error) {
	q := firestoredb.Client.CollectionGroup("designs").
		Where("renderStatus", "==", DesignRenderRendering).
		Where("renderStartedAt", "<", cutoffMs)
	if limit > 0 {
		q = q.Limit(limit)
	}
	iter := q.Documents(ctx)
	defer iter.Stop()

	out := []StuckRender{}
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return out, err
		}
		// brands/{brandId}/contents/{contentId}/designs/{revisionId}
		contentRef := doc.Ref.Parent.Parent
		if contentRef == nil || contentRef.Parent == nil || contentRef.Parent.Parent == nil {
			continue
		}
		started, _ := doc.Data()["renderStartedAt"].(int64)
		out = append(out, StuckRender{
			BrandID:    contentRef.Parent.Parent.ID,
			ContentID:  contentRef.ID,
			RevisionID: doc.Ref.ID,
			StartedAt:  started,
		})
	}
	return out, nil
}
