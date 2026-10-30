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

// ── Composition model: the timeline above designs ────────────────────────────
//
// A design is one self-contained HTML document, and until now it was the whole
// of a video: one document in, one MP4 out. A composition makes the TIMELINE
// the primitive instead — a sequence of clips, of which a whole design may be
// one, alongside footage, stills and (once a model is enabled) generated video.
//
// Revisions live at brands/{brandId}/contents/{contentId}/compositions/{revId},
// immutable like design revisions so revert works, and they reuse the SAME
// render lifecycle field names (renderStatus / renderProgress / renderUrl), so
// the app's existing revision watcher needs no change to follow one.
//
// ⚠️ Nothing has to be stored for an existing video design to render through
// this path. The worker derives a single-clip composition from the design in
// memory, so the composition path became the only path with no migration and
// no documents written. This collection is for compositions somebody AUTHORS.
//
// ── Where the timeline arithmetic lives, and why it is here twice ────────────
// `shared-libs/design/composition-runtime.ts` is the canonical implementation,
// imported by the app's timeline UI and by the render worker. This file holds a
// second copy of the offset/overlap arithmetic in Go, deliberately:
//
//	the stored durationMs is what the render is BILLED on, and a duration that
//	arrives from a client is a duration a client can set to 1ms. So Go computes
//	it and overwrites whatever was sent, on every write.
//
// One authority per concern, then: Go owns the stored numbers, the TS runtime
// owns playback and layout. They are the same arithmetic, and the Go tests use
// the TS tests' vectors so a change to one that is not mirrored fails here.
type Composition struct {
	Width  int `json:"width" firestore:"width"`
	Height int `json:"height" firestore:"height"`
	FPS    int `json:"fps" firestore:"fps"`
	// DurationMs is DERIVED — recomputed by MeasureComposition on every write.
	DurationMs int                `json:"durationMs" firestore:"durationMs"`
	Tracks     []CompositionTrack `json:"tracks" firestore:"tracks"`
}

// Track kinds. "overlay" and "audio" are declared so the schema is final;
// clips on them are rejected until the phase that renders them.
const (
	CompositionTrackVideo   = "video"
	CompositionTrackOverlay = "overlay"
	CompositionTrackAudio   = "audio"
)

type CompositionTrack struct {
	Kind  string            `json:"kind" firestore:"kind"`
	Clips []CompositionClip `json:"clips" firestore:"clips"`
}

type CompositionClip struct {
	// ID is stable within the composition so a UI can key and reorder by it.
	ID string `json:"id" firestore:"id"`
	// StartMs is a DERIVED cache of where this clip starts on the timeline.
	StartMs int `json:"startMs" firestore:"startMs"`
	// DurationMs is this clip's own on-screen length. Authored.
	DurationMs   int                    `json:"durationMs" firestore:"durationMs"`
	TransitionIn *CompositionTransition `json:"transitionIn,omitempty" firestore:"transitionIn,omitempty"`
	Source       CompositionClipSource  `json:"source" firestore:"source"`
}

// Transition kinds. Mirrors the TS runtime: a timeline cross-fades or cuts, and
// the scene-level "push"/"slide" effects have no clip-level equivalent.
const (
	CompositionTransitionCut  = "cut"
	CompositionTransitionFade = "fade"
)

type CompositionTransition struct {
	Kind string `json:"kind" firestore:"kind"`
	// Ms is the overlap with the previous clip. Clamped by MeasureComposition.
	Ms int `json:"ms" firestore:"ms"`
}

// Clip source kinds.
const (
	ClipSourceDesign   = "design"
	ClipSourceFootage  = "footage"
	ClipSourceGenVideo = "genvideo"
	ClipSourceImage    = "image"
)

// CompositionClipSource is a FLAT struct with a `kind` discriminator rather
// than one struct per kind.
//
// Firestore has no union type, and the alternative — a map[string]any the
// handlers pick apart — loses every struct tag and every bit of type checking
// at exactly the layer that decides what gets rendered and what gets charged.
// The per-kind fields each carry omitempty, so a design clip stores three
// fields rather than fifteen. The same choice was made for platform options.
type CompositionClipSource struct {
	Kind string `json:"kind" firestore:"kind"`

	// ── design ──
	RevisionID string `json:"revisionId,omitempty" firestore:"revisionId,omitempty"`
	// SceneIndex renders only that scene. Nil means the whole design, internal
	// scene transitions included — which is what an existing video design is.
	SceneIndex *int `json:"sceneIndex,omitempty" firestore:"sceneIndex,omitempty"`
	// Transparent captures with no background, for an overlay track.
	Transparent bool `json:"transparent,omitempty" firestore:"transparent,omitempty"`

	// ── footage / image / a landed genvideo ──
	// URL is the direct playable/loadable url. An AssetID alone cannot render.
	URL     string `json:"url,omitempty" firestore:"url,omitempty"`
	AssetID string `json:"assetId,omitempty" firestore:"assetId,omitempty"`

	// ── footage ──
	InMs  *int     `json:"inMs,omitempty" firestore:"inMs,omitempty"`
	OutMs *int     `json:"outMs,omitempty" firestore:"outMs,omitempty"`
	Speed *float64 `json:"speed,omitempty" firestore:"speed,omitempty"`
	// Volume is 0..1 on the clip's own audio. Nil or 0 strips it.
	Volume *float64 `json:"volume,omitempty" firestore:"volume,omitempty"`

	// ── image ──
	KenBurns string `json:"kenBurns,omitempty" firestore:"kenBurns,omitempty"`

	// ── genvideo ──
	Model         string `json:"model,omitempty" firestore:"model,omitempty"`
	Prompt        string `json:"prompt,omitempty" firestore:"prompt,omitempty"`
	StartFrameURL string `json:"startFrameUrl,omitempty" firestore:"startFrameUrl,omitempty"`
	EndFrameURL   string `json:"endFrameUrl,omitempty" firestore:"endFrameUrl,omitempty"`
	JobID         string `json:"jobId,omitempty" firestore:"jobId,omitempty"`
}

// ContentCompositionRef is the lightweight pointer cached on the content doc,
// shaped like ContentDesignRef so the app reads the two the same way.
type ContentCompositionRef struct {
	RevisionID string `json:"revisionId" firestore:"revisionId"`
	Width      int    `json:"width" firestore:"width"`
	Height     int    `json:"height" firestore:"height"`
	FPS        int    `json:"fps" firestore:"fps"`
	DurationMs int    `json:"durationMs" firestore:"durationMs"`
	ClipCount  int    `json:"clipCount" firestore:"clipCount"`
	RenderURL  string `json:"renderUrl,omitempty" firestore:"renderUrl,omitempty"`
	UpdatedAt  int64  `json:"updatedAt" firestore:"updatedAt"`
}

// ContentCompositionRevision is one immutable composition snapshot.
//
// The render fields are named exactly as ContentDesignRevision's are — the app
// subscribes to a revision document and reads renderStatus/renderProgress/
// renderUrl without caring which kind of revision it is looking at.
type ContentCompositionRevision struct {
	ID          string      `json:"id" firestore:"-"`
	Composition Composition `json:"composition" firestore:"composition"`

	// Origin: "timeline" | "edit" | "revert".
	Origin           string `json:"origin,omitempty" firestore:"origin,omitempty"`
	ParentRevisionID string `json:"parentRevisionId,omitempty" firestore:"parentRevisionId,omitempty"`

	RenderURL           string  `json:"renderUrl,omitempty" firestore:"renderUrl,omitempty"`
	PosterURL           string  `json:"posterUrl,omitempty" firestore:"posterUrl,omitempty"`
	RenderStatus        string  `json:"renderStatus,omitempty" firestore:"renderStatus,omitempty"`
	RenderProgress      float64 `json:"renderProgress,omitempty" firestore:"renderProgress,omitempty"`
	RenderError         string  `json:"renderError,omitempty" firestore:"renderError,omitempty"`
	RenderJobID         string  `json:"renderJobId,omitempty" firestore:"renderJobId,omitempty"`
	RenderStartedAt     int64   `json:"renderStartedAt,omitempty" firestore:"renderStartedAt,omitempty"`
	RenderedAt          int64   `json:"renderedAt,omitempty" firestore:"renderedAt,omitempty"`
	RenderScale         float64 `json:"renderScale,omitempty" firestore:"renderScale,omitempty"`
	RenderChargedTokens int64   `json:"renderChargedTokens,omitempty" firestore:"renderChargedTokens,omitempty"`
	CreatedAt           int64   `json:"createdAt" firestore:"createdAt"`
}

// Defaults, mirroring composition-runtime.ts.
const (
	DefaultClipMs       = 3000
	DefaultTransitionMs = 400
)

// ── Timeline arithmetic ──────────────────────────────────────────────────────

// MeasuredCompositionClip is one clip resolved onto the composition's clock.
type MeasuredCompositionClip struct {
	Index        int
	StartMs      int
	DurationMs   int
	TransitionMs int
	Kind         string
}

// MeasureComposition resolves every clip onto the composition clock and returns
// the clips plus the real total.
//
// Offsets are DERIVED from the sequence, not read from StartMs. Authoring
// absolute positions would admit a gap — which can only render as black, and
// which nobody intends — and an overlap no transition asked for, where two
// clips claim the same instant and which one wins is a coin flip.
//
// The clamp is the part worth reading, and it is design-runtime's rule
// transplanted: an overlap may never exceed HALF of the shorter of the two
// clips it joins. Without it a transition longer than the outgoing clip would
// push the next clip's start back past it, the offsets would stop increasing,
// and "which clip is at time T" would stop having an answer.
//
// ⚠️ Mirrors measureComposition in shared-libs/design/composition-runtime.ts.
// Change one, change both — the Go tests use the TS tests' vectors so that a
// one-sided change fails rather than drifts.
func MeasureComposition(c *Composition) ([]MeasuredCompositionClip, int) {
	clips := compositionVideoClips(c)
	if len(clips) == 0 {
		return nil, 0
	}

	durations := make([]int, len(clips))
	overlaps := make([]int, len(clips))
	for i := range clips {
		durations[i] = clips[i].DurationMs
		if durations[i] <= 0 {
			durations[i] = DefaultClipMs
		}
		if i == 0 || clips[i].TransitionIn == nil || clips[i].TransitionIn.Kind != CompositionTransitionFade {
			continue
		}
		ms := clips[i].TransitionIn.Ms
		if ms <= 0 {
			ms = DefaultTransitionMs
		}
		if half := min(durations[i-1], durations[i]) / 2; ms > half {
			ms = half
		}
		overlaps[i] = ms
	}

	out := make([]MeasuredCompositionClip, len(clips))
	cursor := 0
	for i := range clips {
		start := 0
		if i > 0 {
			start = cursor - overlaps[i]
		}
		out[i] = MeasuredCompositionClip{
			Index:        i,
			StartMs:      start,
			DurationMs:   durations[i],
			TransitionMs: overlaps[i],
			Kind:         clips[i].Source.Kind,
		}
		cursor = start + durations[i]
	}
	return out, cursor
}

// compositionVideoClips returns the clips on the one video track.
func compositionVideoClips(c *Composition) []CompositionClip {
	if c == nil {
		return nil
	}
	for _, t := range c.Tracks {
		if t.Kind == CompositionTrackVideo {
			return t.Clips
		}
	}
	return nil
}

// StampDerived recomputes the cached offsets and total in place, so what is
// stored can never contradict what will be rendered — or what was billed.
func (c *Composition) StampDerived() {
	measured, total := MeasureComposition(c)
	c.DurationMs = total
	for ti := range c.Tracks {
		if c.Tracks[ti].Kind != CompositionTrackVideo {
			continue
		}
		for i := range c.Tracks[ti].Clips {
			if i < len(measured) {
				c.Tracks[ti].Clips[i].StartMs = measured[i].StartMs
			}
		}
		// Only the first video track is measured; a second is a validation error.
		break
	}
}

// CompositionProblem is one reason a composition cannot be rendered, or should
// be warned about. Path is dotted so a UI can point at the offending field.
type CompositionProblem struct {
	Path    string `json:"path"`
	Message string `json:"message"`
	Fatal   bool   `json:"fatal"`
}

// ValidateComposition checks a composition before anything spends money on it.
//
// ⚠️ Mirrors validateComposition in composition-runtime.ts, and for the same
// reason the arithmetic is mirrored: this is the copy that runs before a charge,
// so it cannot depend on a client having run the other one.
func ValidateComposition(c *Composition) []CompositionProblem {
	var problems []CompositionProblem
	add := func(path, message string, fatal bool) {
		problems = append(problems, CompositionProblem{Path: path, Message: message, Fatal: fatal})
	}
	if c == nil {
		add("composition", "There is no composition to render.", true)
		return problems
	}

	if c.Width <= 0 || c.Height <= 0 {
		add("size", "A composition needs a positive width and height.", true)
	}
	if c.FPS <= 0 || c.FPS > 60 {
		add("fps", "Frame rate must be between 1 and 60.", true)
	}

	videoTracks := 0
	for _, t := range c.Tracks {
		switch t.Kind {
		case CompositionTrackVideo:
			videoTracks++
		case CompositionTrackOverlay:
			if len(t.Clips) > 0 {
				add("tracks.overlay", "Overlay tracks aren't supported yet.", true)
			}
		case CompositionTrackAudio:
			if len(t.Clips) > 0 {
				add("tracks.audio", "Placed audio clips aren't supported yet.", true)
			}
		default:
			add("tracks.kind", fmt.Sprintf("Unknown track kind %q.", t.Kind), true)
		}
	}
	if videoTracks == 0 {
		add("tracks", "A composition needs a video track.", true)
	}
	if videoTracks > 1 {
		add("tracks", "A composition can only have one video track.", true)
	}

	clips := compositionVideoClips(c)
	if videoTracks == 1 && len(clips) == 0 {
		add("tracks.video.clips", "A composition needs at least one clip.", true)
	}

	seen := map[string]bool{}
	for i := range clips {
		clip := clips[i]
		at := fmt.Sprintf("tracks.video.clips[%d]", i)
		switch {
		case clip.ID == "":
			add(at+".id", "Every clip needs an id.", true)
		case seen[clip.ID]:
			add(at+".id", fmt.Sprintf("Duplicate clip id %q.", clip.ID), true)
		default:
			seen[clip.ID] = true
		}
		if clip.DurationMs <= 0 {
			add(at+".durationMs", "A clip needs a duration.", true)
		}

		s := clip.Source
		switch s.Kind {
		case ClipSourceDesign:
			if s.RevisionID == "" {
				add(at+".source.revisionId", "A design clip needs a revision.", true)
			}
			if s.SceneIndex != nil && *s.SceneIndex < 0 {
				add(at+".source.sceneIndex", "A scene index can't be negative.", true)
			}
			if s.Transparent {
				add(at+".source.transparent", "Transparent design clips aren't supported yet.", true)
			}
		case ClipSourceFootage:
			if s.URL == "" {
				add(at+".source.url", "A footage clip needs a url.", true)
			}
			if s.Speed != nil && *s.Speed <= 0 {
				add(at+".source.speed", "Speed must be greater than zero.", true)
			}
			if s.InMs != nil && s.OutMs != nil && *s.OutMs <= *s.InMs {
				add(at+".source.outMs", "A trim must end after it starts.", true)
			}
		case ClipSourceImage:
			if s.URL == "" {
				add(at+".source.url", "An image clip needs a url.", true)
			}
		case ClipSourceGenVideo:
			// Not fatal by kind but fatal in practice: a clip still waiting on
			// its generation has nothing to render, and saying so beats
			// encoding a black gap where the shot should be.
			if s.URL == "" {
				add(at+".source.url", "This generated clip hasn't finished yet, so it can't be rendered.", true)
			}
		case "":
			add(at+".source", "A clip needs a source.", true)
		default:
			add(at+".source.kind", fmt.Sprintf("Unknown clip source %q.", s.Kind), true)
		}

		if clip.TransitionIn != nil && clip.TransitionIn.Kind == CompositionTransitionFade && i == 0 {
			add(at+".transitionIn", "The first clip can't fade in from anything.", false)
		}
	}
	return problems
}

// CompositionRenderable reports whether nothing fatal stands between this
// composition and a render, with the first blocking message for the caller to
// show. The message is user-facing, so it reads as a sentence.
func CompositionRenderable(c *Composition) (bool, string) {
	for _, p := range ValidateComposition(c) {
		if p.Fatal {
			return false, p.Message
		}
	}
	return true, ""
}

// SingleDesignPassthrough returns the design revision id when this composition
// is exactly one whole design and nothing else.
//
// That case describes precisely what every video design was before there were
// compositions, and it must take the pre-composition render path verbatim: a
// concat or an intermediate re-encode of a single clip is a second lossy
// generation, so the output would be equivalent but not identical.
//
// ⚠️ Mirrors singleDesignPassthrough in composition-runtime.ts.
func SingleDesignPassthrough(c *Composition) string {
	if c == nil {
		return ""
	}
	videoTracks := 0
	for _, t := range c.Tracks {
		if t.Kind == CompositionTrackVideo {
			videoTracks++
			continue
		}
		if len(t.Clips) > 0 {
			return ""
		}
	}
	if videoTracks != 1 {
		return ""
	}

	// The MEASURED transition, not the authored one, so this cannot disagree
	// with the arithmetic that will be rendered.
	measured, _ := MeasureComposition(c)
	if len(measured) != 1 || measured[0].TransitionMs > 0 {
		return ""
	}

	s := compositionVideoClips(c)[0].Source
	if s.Kind != ClipSourceDesign || s.SceneIndex != nil || s.Transparent {
		return ""
	}
	return s.RevisionID
}

// CompositionFromDesign builds the implicit composition for a content that has
// a design and no timeline of its own.
//
// This is the adapter that lets every pre-existing video design render THROUGH
// the composition path with no migration and no UI change: the timeline is the
// primitive everywhere, and one whole design is a passthrough rather than a
// special case bolted beside it.
func CompositionFromDesign(revisionID string, width, height, fps, durationMs int) *Composition {
	if durationMs <= 0 {
		durationMs = DefaultClipMs
	}
	c := &Composition{
		Width:  width,
		Height: height,
		FPS:    fps,
		Tracks: []CompositionTrack{{
			Kind: CompositionTrackVideo,
			Clips: []CompositionClip{{
				ID:         "design",
				DurationMs: durationMs,
				Source:     CompositionClipSource{Kind: ClipSourceDesign, RevisionID: revisionID},
			}},
		}},
	}
	c.StampDerived()
	return c
}

// ── Firestore ────────────────────────────────────────────────────────────────

func contentCompositionsCollection(brandID, contentID string) *firestore.CollectionRef {
	return firestoredb.Client.Collection(
		fmt.Sprintf("brands/%s/contents/%s/compositions", brandID, contentID))
}

// CreateCompositionRevision writes a new immutable composition revision,
// stamping the derived offsets first so the stored numbers are ours.
func CreateCompositionRevision(brandID, contentID string, rev *ContentCompositionRevision) (string, error) {
	rev.Composition.StampDerived()
	if rev.CreatedAt == 0 {
		rev.CreatedAt = time.Now().UnixMilli()
	}
	ref, _, err := contentCompositionsCollection(brandID, contentID).Add(context.Background(), rev)
	if err != nil {
		return "", fmt.Errorf("CreateCompositionRevision: %w", err)
	}
	return ref.ID, nil
}

// GetCompositionRevision reads one revision, or (nil, nil) if absent.
func GetCompositionRevision(brandID, contentID, revisionID string) (*ContentCompositionRevision, error) {
	doc, err := contentCompositionsCollection(brandID, contentID).Doc(revisionID).Get(context.Background())
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}
	var r ContentCompositionRevision
	if err := doc.DataTo(&r); err != nil {
		return nil, err
	}
	r.ID = doc.Ref.ID
	// Recomputed on read as well as on write: a revision written before a
	// change to the arithmetic would otherwise keep quoting the old total to
	// the pricing call that reads it.
	r.Composition.StampDerived()
	return &r, nil
}

// ListCompositionRevisions returns a content's composition revisions newest-first.
func ListCompositionRevisions(brandID, contentID string, limit int) ([]ContentCompositionRevision, error) {
	q := contentCompositionsCollection(brandID, contentID).OrderBy("createdAt", firestore.Desc)
	if limit > 0 {
		q = q.Limit(limit)
	}
	iter := q.Documents(context.Background())
	defer iter.Stop()
	out := []ContentCompositionRevision{}
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var r ContentCompositionRevision
		if err := doc.DataTo(&r); err != nil {
			return nil, err
		}
		r.ID = doc.Ref.ID
		r.Composition.StampDerived()
		out = append(out, r)
	}
	return out, nil
}

// SetContentCompositionRef points the content at a composition revision.
func SetContentCompositionRef(brandID, contentID string, ref *ContentCompositionRef) error {
	ref.UpdatedAt = time.Now().UnixMilli()
	return UpdateContentFields(brandID, contentID, map[string]interface{}{
		"compositionRef": ref,
	})
}

// CompositionRefFrom builds the cached pointer for a stored revision.
func CompositionRefFrom(revisionID string, c *Composition) *ContentCompositionRef {
	measured, total := MeasureComposition(c)
	return &ContentCompositionRef{
		RevisionID: revisionID,
		Width:      c.Width,
		Height:     c.Height,
		FPS:        c.FPS,
		DurationMs: total,
		ClipCount:  len(measured),
	}
}

// updateCompositionRevisionFields merges fields into one revision doc. Every
// render write goes through here, so the worker never touches firestoredb.Client.
func updateCompositionRevisionFields(brandID, contentID, revisionID string, fields map[string]interface{}) error {
	_, err := contentCompositionsCollection(brandID, contentID).
		Doc(revisionID).
		Set(context.Background(), fields, firestore.MergeAll)
	if err != nil {
		return fmt.Errorf("updateCompositionRevisionFields(%s): %w", revisionID, err)
	}
	return nil
}

// MarkCompositionRenderQueued stamps a revision the moment a render is
// submitted. renderStartedAt is SET, not cleared, for the same reason it is on
// a design revision: ListStuckRenders uses a range filter, and a Firestore range
// filter skips documents missing the field entirely — so a job that dies before
// any worker touches it would sit in "queued" forever with no way out.
func MarkCompositionRenderQueued(brandID, contentID, revisionID, jobID string) error {
	return updateCompositionRevisionFields(brandID, contentID, revisionID, map[string]interface{}{
		"renderStatus":    DesignRenderQueued,
		"renderJobId":     jobID,
		"renderError":     firestore.Delete,
		"renderProgress":  firestore.Delete,
		"renderStartedAt": time.Now().UnixMilli(),
	})
}

// MarkCompositionRenderStarted is called by the worker when it picks the job up.
func MarkCompositionRenderStarted(brandID, contentID, revisionID, jobID string) error {
	fields := map[string]interface{}{
		"renderStatus":    DesignRenderRendering,
		"renderStartedAt": time.Now().UnixMilli(),
	}
	if jobID != "" {
		fields["renderJobId"] = jobID
	}
	return updateCompositionRevisionFields(brandID, contentID, revisionID, fields)
}

// SetCompositionRenderProgress reports 0..1 during a render.
func SetCompositionRenderProgress(brandID, contentID, revisionID string, progress float64) error {
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	return updateCompositionRevisionFields(brandID, contentID, revisionID, map[string]interface{}{
		"renderProgress": progress,
	})
}

// MarkCompositionRenderFailed records a user-facing reason, shown verbatim.
func MarkCompositionRenderFailed(brandID, contentID, revisionID, reason string) error {
	return updateCompositionRevisionFields(brandID, contentID, revisionID, map[string]interface{}{
		"renderStatus":   DesignRenderFailed,
		"renderError":    reason,
		"renderProgress": firestore.Delete,
	})
}

// SetCompositionRenderResult records a finished render on the revision. Written
// last, after the upload has succeeded — it is the field that separates "a
// timeline exists" from "a publishable asset exists".
func SetCompositionRenderResult(brandID, contentID, revisionID, renderURL, posterURL string, scale float64) error {
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
	return updateCompositionRevisionFields(brandID, contentID, revisionID, fields)
}

// MarkCompositionRenderCharged records what the wallet was debited, so a retry
// after a failure is not charged twice.
func MarkCompositionRenderCharged(brandID, contentID, revisionID string, tokens int64) error {
	return updateCompositionRevisionFields(brandID, contentID, revisionID, map[string]interface{}{
		"renderChargedTokens": tokens,
	})
}

// SetContentCompositionRenderOutput publishes a composition render onto the
// content: `attachments` is what the publish pipeline reads, and the cached
// compositionRef.renderUrl is what share previews use.
func SetContentCompositionRenderOutput(brandID, contentID, renderURL, posterURL string) error {
	attachment := map[string]interface{}{"type": "video", "playUrl": renderURL}
	if posterURL != "" {
		attachment["thumbUrl"] = posterURL
	}
	return UpdateContentFields(brandID, contentID, map[string]interface{}{
		"compositionRef": map[string]interface{}{"renderUrl": renderURL},
		"attachments":    []interface{}{attachment},
	})
}

// ListStuckCompositionRenders finds composition revisions sitting in `status`
// since before `cutoffMs`, so the reconcile cron can close them.
//
// The design-revision sweep cannot cover these: it is a collection-group query
// over `designs`, and a composition lives in `compositions`. Two collection
// groups, two queries — and the same composite index shape, which ships in both
// index files.
func ListStuckCompositionRenders(ctx context.Context, status string, cutoffMs int64, limit int) ([]StuckRender, error) {
	q := firestoredb.Client.CollectionGroup("compositions").
		Where("renderStatus", "==", status).
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
		// brands/{brandId}/contents/{contentId}/compositions/{revisionId}
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
