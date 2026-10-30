package rendering

import (
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

// ── What is this content's current media, and what does rendering it cost? ───
//
// Two things need that answer and must never disagree about it: the Render
// button, and the publish gate that renders a scheduled post before it goes
// out. They used to ask the question separately, which was fine while a content
// had exactly one kind of renderable media. It now has two, and a handler that
// renders the timeline while the gate checks the design would publish media
// nobody approved — so the precedence rule lives here, once.
type RenderTarget struct {
	// Kind is "design" or "composition" — which subcollection the revision is
	// in, and therefore which document carries its render status.
	Kind string
	// RevisionID identifies the revision AND the status document.
	RevisionID string
	// DocType is what the worker is being asked to produce: "image", "video"
	// or "composition".
	DocType string

	RenderURL     string
	RenderStatus  string
	ChargedTokens int64

	// Problem is a user-facing sentence when this target cannot be rendered at
	// all — a clip with no media, a generation still in flight. Empty means
	// there is nothing in the way. Only compositions can be unrenderable; a
	// design revision either has HTML or does not exist.
	Problem string

	// Tokens is what this render will cost the wallet.
	Tokens int64
}

const (
	renderKindDesign      = "design"
	renderKindComposition = "composition"
)

// ResolveRenderTarget returns what should be rendered for a content, or nil
// when it has no renderable media at all.
//
// A composition WINS over a design when both exist. A composition is authored
// on top of designs — it is the thing the user arranged, and the design is one
// clip inside it — so once a timeline exists it is the content's media. The
// reverse precedence would silently publish a single scene of a multi-clip reel.
func ResolveRenderTarget(brandID, contentID string, ct *trendlymodels.Content) (*RenderTarget, error) {
	if ct == nil {
		return nil, nil
	}

	if ct.CompositionRef != nil && ct.CompositionRef.RevisionID != "" {
		rev, err := trendlymodels.GetCompositionRevision(brandID, contentID, ct.CompositionRef.RevisionID)
		if err != nil {
			return nil, err
		}
		if rev != nil {
			// The duration and clip count come from the STORED composition,
			// recomputed on read, never from the cached ref — the ref is
			// client-writable and this number is what gets charged.
			clips, total := trendlymodels.MeasureComposition(&rev.Composition)
			// Checked HERE rather than in the worker alone, so a timeline with a
			// clip that cannot render is refused before the wallet is debited
			// for an attempt that was never going to produce anything.
			_, why := trendlymodels.CompositionRenderable(&rev.Composition)
			return &RenderTarget{
				Kind:          renderKindComposition,
				RevisionID:    rev.ID,
				DocType:       renderKindComposition,
				RenderURL:     rev.RenderURL,
				RenderStatus:  rev.RenderStatus,
				ChargedTokens: rev.RenderChargedTokens,
				Problem:       why,
				Tokens: trendlymodels.CompositionRenderTokenCost(
					total, len(clips), trendlymodels.CompositionDecodedMs(&rev.Composition)),
			}, nil
		}
		// A compositionRef pointing at a revision that is gone falls through to
		// the design rather than failing: the design is still publishable media,
		// and refusing to render anything would be a worse answer.
	}

	if ct.DesignRef == nil || ct.DesignRef.RevisionID == "" {
		return nil, nil
	}
	rev, err := trendlymodels.GetDesignRevision(brandID, contentID, ct.DesignRef.RevisionID)
	if err != nil || rev == nil {
		return nil, err
	}
	return DesignRenderTarget(rev), nil
}

// DesignRenderTarget describes a render of one design revision, for the callers
// that already have the revision in hand — re-rendering an older revision by id,
// for instance, which deliberately bypasses the composition precedence.
func DesignRenderTarget(rev *trendlymodels.ContentDesignRevision) *RenderTarget {
	return &RenderTarget{
		Kind:          renderKindDesign,
		RevisionID:    rev.ID,
		DocType:       rev.DocType,
		RenderURL:     rev.RenderURL,
		RenderStatus:  rev.RenderStatus,
		ChargedTokens: rev.RenderChargedTokens,
		Tokens:        trendlymodels.RenderTokenCost(rev.DocType, rev.SlideCount, rev.DurationMs),
	}
}

// InFlight reports whether a render for this target is already running, so a
// caller can wait rather than start a second one.
func (t *RenderTarget) InFlight() bool {
	return t.RenderStatus == trendlymodels.DesignRenderQueued ||
		t.RenderStatus == trendlymodels.DesignRenderRendering
}

// markQueued / markFailed route to the right subcollection. The two revision
// kinds carry identical render fields, so only the model function differs.
func (t *RenderTarget) markQueued(brandID, contentID, jobID string) error {
	if t.Kind == renderKindComposition {
		return trendlymodels.MarkCompositionRenderQueued(brandID, contentID, t.RevisionID, jobID)
	}
	return trendlymodels.MarkDesignRenderQueued(brandID, contentID, t.RevisionID, jobID)
}

func (t *RenderTarget) markFailed(brandID, contentID, reason string) error {
	if t.Kind == renderKindComposition {
		return trendlymodels.MarkCompositionRenderFailed(brandID, contentID, t.RevisionID, reason)
	}
	return trendlymodels.MarkDesignRenderFailed(brandID, contentID, t.RevisionID, reason)
}

func (t *RenderTarget) markCharged(brandID, contentID string, tokens int64) error {
	if t.Kind == renderKindComposition {
		return trendlymodels.MarkCompositionRenderCharged(brandID, contentID, t.RevisionID, tokens)
	}
	return trendlymodels.MarkDesignRenderCharged(brandID, contentID, t.RevisionID, tokens)
}
