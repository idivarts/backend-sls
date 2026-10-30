package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// ── Timeline tools: the AI can place footage, not just read it ───────────────
//
// `get_footage` already let the model see what is IN a brand's video — its
// length, what is said and when, a shot list of what is on screen. It could
// therefore answer "which bit is the good bit" in prose and do nothing about
// it, which is the gap these two tools close.
//
// `cut_footage` is deliberately NARROW rather than a general "write me a
// timeline". A tool that accepts a whole composition hands the model the
// opportunity to clobber a timeline somebody arranged by hand, and to invent
// clip kinds, urls and durations that the renderer will refuse after the wallet
// has been charged. This one takes the only thing the model is actually good at
// — WHICH WINDOWS OF A TAKE ARE WORTH KEEPING, in source time, read off the
// transcript and the shot list — and the backend builds a valid composition
// around them.
//
// Everything else about the timeline stays the user's: trim nudges, speed,
// volume, transitions and order are one tap each in the inspector, and a
// composition is an immutable revision, so an AI cut that overshoots is undone
// by reverting rather than argued with.

const (
	toolGetTimeline = "get_timeline"
	toolCutFootage  = "cut_footage"
)

// The shortest piece worth keeping: two frames at 30fps, matching MIN_CLIP_MS
// in the app's composition-edits. A fade needs something to fade.
const minPieceMs = 67

// A cut of more than this many pieces is not an edit, it is a strobe — and each
// piece is an FFmpeg input and an intermediate encode, so the cap is also what
// keeps one tool call from producing a render that times out.
const maxCutPieces = 12

func timelineServerTools() []openrouter.Tool {
	return []openrouter.Tool{
		openrouter.NewFunctionTool(
			toolGetTimeline,
			"Read this content's video timeline: the clips in order, what each one is "+
				"(the design, uploaded footage, a still, a generated clip), how long each is on "+
				"screen, and how it enters. Call this BEFORE cut_footage so you know what you "+
				"would be replacing — a content with no authored timeline reports the single "+
				"design clip every video starts as.",
			openrouter.ObjectSchema(map[string]any{}, []string{}),
		),
		openrouter.NewFunctionTool(
			toolCutFootage,
			"Cut uploaded footage into this content's video by choosing which windows of the "+
				"take to keep. Call get_footage first to read the transcript and the shot list, "+
				"then pass the pieces you want IN THE ORDER THEY SHOULD PLAY — they do not have "+
				"to be in the order they were filmed.\n"+
				"`startMs`/`endMs` are milliseconds into the SOURCE video, the same clock the "+
				"transcript words and the shots use. Cut on the gaps between sentences, not "+
				"mid-word: the transcript's word timings are there for exactly that.\n"+
				"This REPLACES the content's picture track. Use `keepDesign` to keep the "+
				"existing design as an opening or closing card rather than discarding it. "+
				"Trimming, speed, volume, transitions and order are all editable by hand "+
				"afterwards, so prefer a clean, confident cut over a cautious one.",
			openrouter.ObjectSchema(map[string]any{
				"assetId": openrouter.StringProp(
					"The footage asset to cut, from get_footage. Required."),
				"pieces": map[string]any{
					"type": "array",
					"description": fmt.Sprintf(
						"The windows to keep, in playing order (1-%d).", maxCutPieces),
					"items": openrouter.ObjectSchema(map[string]any{
						"startMs": openrouter.NumberProp("Where this piece starts in the source video."),
						"endMs":   openrouter.NumberProp("Where it ends in the source video."),
						"speed":   openrouter.NumberProp("Optional playback rate. 1 is untouched, 2 is double speed."),
						"keepAudio": map[string]any{
							"type": "boolean",
							"description": "Keep this piece's own sound. Default false — footage is " +
								"silent unless the speech in it is the point, so it cannot talk over " +
								"the music bed or the voiceover.",
						},
					}, []string{"startMs", "endMs"}),
				},
				"keepDesign": openrouter.StringProp(
					"What to do with the existing design: \"none\" (default), \"before\" to open on it, " +
						"\"after\" to end on it."),
				"transition": openrouter.StringProp(
					"How the pieces join: \"cut\" (default) or \"fade\"."),
			}, []string{"assetId", "pieces"}),
		),
	}
}

func runGetTimeline(ctx context.Context, brandID, contentID string) (string, error) {
	if contentID == "" {
		return jsonResult(map[string]any{"ok": false, "error": "no content is open"}), nil
	}
	content, err := trendlymodels.GetContent(brandID, contentID)
	if err != nil || content == nil {
		return jsonResult(map[string]any{"ok": false, "error": "content not found"}), nil
	}

	composition, authored, err := currentComposition(brandID, contentID, content)
	if err != nil {
		return "", err
	}
	if composition == nil {
		return jsonResult(map[string]any{
			"ok": true, "hasTimeline": false,
			"note": "This content has no video design, so there is nothing on a timeline yet.",
		}), nil
	}
	return jsonResult(map[string]any{
		"ok":          true,
		"hasTimeline": true,
		"authored":    authored,
		"note": "`authored` false means nobody has arranged clips — this is the single design " +
			"clip every video starts as, and replacing it loses nothing.",
		"timeline": describeComposition(composition),
	}), nil
}

type cutPiece struct {
	StartMs   int      `json:"startMs"`
	EndMs     int      `json:"endMs"`
	Speed     *float64 `json:"speed"`
	KeepAudio bool     `json:"keepAudio"`
}

type cutFootageArgs struct {
	AssetID    string     `json:"assetId"`
	Pieces     []cutPiece `json:"pieces"`
	KeepDesign string     `json:"keepDesign"`
	Transition string     `json:"transition"`
}

func runCutFootage(ctx context.Context, brandID, contentID, arguments string) (string, error) {
	if contentID == "" {
		return jsonResult(map[string]any{"ok": false, "error": "no content is open"}), nil
	}
	var a cutFootageArgs
	if err := json.Unmarshal([]byte(arguments), &a); err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "could not read the arguments"}), nil
	}
	if strings.TrimSpace(a.AssetID) == "" || len(a.Pieces) == 0 {
		return jsonResult(map[string]any{
			"ok":    false,
			"error": "an assetId and at least one piece are required",
		}), nil
	}
	if len(a.Pieces) > maxCutPieces {
		return jsonResult(map[string]any{
			"ok": false,
			"error": fmt.Sprintf(
				"%d pieces is too many — keep it to %d or fewer and make each one count",
				len(a.Pieces), maxCutPieces),
		}), nil
	}

	content, err := trendlymodels.GetContent(brandID, contentID)
	if err != nil || content == nil {
		return jsonResult(map[string]any{"ok": false, "error": "content not found"}), nil
	}
	asset, err := trendlymodels.GetAsset(ctx, brandID, a.AssetID)
	if err != nil {
		return "", err
	}
	if asset == nil || asset.Kind != trendlymodels.AssetKindVideo {
		return jsonResult(map[string]any{
			"ok":    false,
			"error": "that asset is not a video in this brand's library — call get_footage for the ids",
		}), nil
	}
	if asset.URL == "" {
		return jsonResult(map[string]any{"ok": false, "error": "that clip has no playable url"}), nil
	}

	// The source's length, for clamping. Ingest's measurement is preferred: the
	// asset's own durationMs can come from an upload that reported nothing.
	sourceMs := asset.DurationMs
	if asset.Ingest != nil && asset.Ingest.DurationMs > 0 {
		sourceMs = asset.Ingest.DurationMs
	}

	base, _, err := currentComposition(brandID, contentID, content)
	if err != nil {
		return "", err
	}
	if base == nil {
		return jsonResult(map[string]any{
			"ok":    false,
			"error": "this content has no video design to build a timeline on",
		}), nil
	}

	fade := strings.EqualFold(a.Transition, "fade")
	clips := make([]trendlymodels.CompositionClip, 0, len(a.Pieces)+1)

	// The design, kept as an opening card. Taken from the base composition so
	// it carries the revision the content is actually pointing at.
	designClip := firstDesignClip(base)
	if strings.EqualFold(a.KeepDesign, "before") && designClip != nil {
		clips = append(clips, *designClip)
	}

	var skipped []string
	for i, p := range a.Pieces {
		start, end := clampWindow(p.StartMs, p.EndMs, sourceMs)
		if end-start < minPieceMs {
			// Reported rather than silently dropped: a model that asked for a
			// 20ms piece has misread the timings, and it needs to know.
			skipped = append(skipped, fmt.Sprintf(
				"piece %d (%dms–%dms) is too short or outside the video", i+1, p.StartMs, p.EndMs))
			continue
		}
		speed := 1.0
		if p.Speed != nil && *p.Speed > 0 {
			speed = *p.Speed
		}
		volume := 0.0
		if p.KeepAudio {
			volume = 1.0
		}
		inMs, outMs := start, end
		source := trendlymodels.CompositionClipSource{
			Kind:    trendlymodels.ClipSourceFootage,
			URL:     asset.URL,
			AssetID: asset.ID,
			InMs:    &inMs,
			OutMs:   &outMs,
			Speed:   &speed,
			Volume:  &volume,
		}
		clip := trendlymodels.CompositionClip{
			ID: fmt.Sprintf("cut%d", len(clips)+1),
			// The window divided by the speed: at 2x a three-second window is on
			// screen for a second and a half. The renderer emits DurationMs
			// frames and decodes the window, so the two have to agree or the
			// clip ends on a freeze.
			DurationMs: onScreenMs(end-start, speed),
			Source:     source,
		}
		if fade && len(clips) > 0 {
			clip.TransitionIn = &trendlymodels.CompositionTransition{
				Kind: trendlymodels.CompositionTransitionFade,
				Ms:   trendlymodels.DefaultTransitionMs,
			}
		}
		clips = append(clips, clip)
	}

	if len(clips) == 0 || !hasFootage(clips) {
		return jsonResult(map[string]any{
			"ok":      false,
			"error":   "none of those pieces could be used",
			"skipped": skipped,
		}), nil
	}

	if strings.EqualFold(a.KeepDesign, "after") && designClip != nil {
		end := *designClip
		if fade {
			end.TransitionIn = &trendlymodels.CompositionTransition{
				Kind: trendlymodels.CompositionTransitionFade,
				Ms:   trendlymodels.DefaultTransitionMs,
			}
		}
		clips = append(clips, end)
	}

	next := &trendlymodels.Composition{
		Width:  base.Width,
		Height: base.Height,
		FPS:    base.FPS,
		Tracks: []trendlymodels.CompositionTrack{
			{Kind: trendlymodels.CompositionTrackVideo, Clips: clips},
		},
	}
	// Overlays survive the cut: a caption track belongs to the video, not to
	// the footage underneath it. They are timed in composition time, though, so
	// a cut that changes the length will have moved the picture under them —
	// which is why the result says so rather than pretending otherwise.
	movedOverlays := 0
	for _, t := range base.Tracks {
		if t.Kind == trendlymodels.CompositionTrackOverlay && len(t.Clips) > 0 {
			next.Tracks = append(next.Tracks, t)
			movedOverlays += len(t.Clips)
		}
	}
	next.StampDerived()

	if ok, why := trendlymodels.CompositionRenderable(next); !ok {
		return jsonResult(map[string]any{"ok": false, "error": why}), nil
	}

	revID, err := trendlymodels.CreateCompositionRevision(brandID, contentID,
		&trendlymodels.ContentCompositionRevision{
			Composition:      *next,
			Origin:           "timeline",
			ParentRevisionID: parentRevisionID(content),
		})
	if err != nil {
		return "", err
	}
	if err := trendlymodels.SetContentCompositionRef(brandID, contentID,
		trendlymodels.CompositionRefFrom(revID, next)); err != nil {
		return "", err
	}

	out := map[string]any{
		"ok":         true,
		"revisionId": revID,
		"timeline":   describeComposition(next),
		"note": "The timeline is updated and visible in the app. It has NOT been rendered — " +
			"the user renders when they are happy with it.",
	}
	if len(skipped) > 0 {
		out["skipped"] = skipped
	}
	if movedOverlays > 0 {
		out["overlays"] = fmt.Sprintf(
			"%d overlay(s) were kept at their original times; the picture under them has changed, so check they still land where intended",
			movedOverlays)
	}
	return jsonResult(out), nil
}

// currentComposition returns the content's timeline — stored if one was
// authored, otherwise the implicit single-design one every video starts as.
func currentComposition(brandID, contentID string, content *trendlymodels.Content) (*trendlymodels.Composition, bool, error) {
	if content.CompositionRef != nil && content.CompositionRef.RevisionID != "" {
		rev, err := trendlymodels.GetCompositionRevision(brandID, contentID, content.CompositionRef.RevisionID)
		if err != nil {
			return nil, false, err
		}
		if rev != nil {
			return &rev.Composition, true, nil
		}
	}
	if content.DesignRef == nil || content.DesignRef.RevisionID == "" {
		return nil, false, nil
	}
	rev, err := trendlymodels.GetDesignRevision(brandID, contentID, content.DesignRef.RevisionID)
	if err != nil || rev == nil {
		return nil, false, err
	}
	if rev.DocType != "video" {
		return nil, false, nil
	}
	return trendlymodels.CompositionFromDesign(
		rev.ID, rev.Width, rev.Height, compositionFPS, rev.DurationMs), false, nil
}

// compositionFPS matches the worker's RENDER_FPS default and the app's
// COMPOSITION_FPS. A timeline claiming another rate would re-time every design
// clip inside it.
const compositionFPS = 30

func firstDesignClip(c *trendlymodels.Composition) *trendlymodels.CompositionClip {
	for _, t := range c.Tracks {
		if t.Kind != trendlymodels.CompositionTrackVideo {
			continue
		}
		for i := range t.Clips {
			if t.Clips[i].Source.Kind == trendlymodels.ClipSourceDesign {
				clip := t.Clips[i]
				// A kept design opens or closes the cut, so it enters on a cut
				// unless the caller asked for fades; whatever it used to enter
				// on described a different sequence.
				clip.TransitionIn = nil
				return &clip
			}
		}
	}
	return nil
}

func hasFootage(clips []trendlymodels.CompositionClip) bool {
	for _, c := range clips {
		if c.Source.Kind == trendlymodels.ClipSourceFootage {
			return true
		}
	}
	return false
}

// clampWindow keeps a requested window inside the source video.
//
// A model reading a shot list can round past the end of the take, and a clip
// whose out point is past the end renders as a freeze — so the window is
// clamped here rather than discovered at render time.
func clampWindow(start, end, sourceMs int) (int, int) {
	if start < 0 {
		start = 0
	}
	if end < start {
		start, end = end, start
	}
	if sourceMs > 0 {
		if start > sourceMs {
			start = sourceMs
		}
		if end > sourceMs {
			end = sourceMs
		}
	}
	return start, end
}

func onScreenMs(windowMs int, speed float64) int {
	if speed <= 0 {
		speed = 1
	}
	ms := int(float64(windowMs) / speed)
	if ms < minPieceMs {
		return minPieceMs
	}
	return ms
}

func parentRevisionID(content *trendlymodels.Content) string {
	if content.CompositionRef != nil {
		return content.CompositionRef.RevisionID
	}
	return ""
}

// describeComposition is what the model is told about a timeline.
//
// Measured, not read off the stored fields: offsets and the total are derived,
// and quoting a cached number the renderer disagrees with is how a model ends
// up confidently describing a video nobody will see.
func describeComposition(c *trendlymodels.Composition) map[string]any {
	measured, total := trendlymodels.MeasureComposition(c)
	clips := make([]map[string]any, 0, len(measured))
	videoClips := videoTrackClips(c)
	for _, m := range measured {
		row := map[string]any{
			"index":      m.Index,
			"kind":       m.Kind,
			"startMs":    m.StartMs,
			"durationMs": m.DurationMs,
		}
		if m.TransitionMs > 0 {
			row["entersWith"] = fmt.Sprintf("fade %dms", m.TransitionMs)
		} else {
			row["entersWith"] = "cut"
		}
		if m.Index < len(videoClips) {
			src := videoClips[m.Index].Source
			if src.AssetID != "" {
				row["assetId"] = src.AssetID
			}
			if src.InMs != nil && src.OutMs != nil {
				row["sourceWindowMs"] = []int{*src.InMs, *src.OutMs}
			}
			if src.Speed != nil && *src.Speed != 1 {
				row["speed"] = *src.Speed
			}
			if src.Volume != nil && *src.Volume > 0 {
				row["ownSound"] = *src.Volume
			}
			if src.Kind == trendlymodels.ClipSourceGenVideo && src.URL == "" {
				row["note"] = "still generating — cannot render yet"
			}
		}
		clips = append(clips, row)
	}
	out := map[string]any{
		"durationMs": total,
		"clipCount":  len(clips),
		"clips":      clips,
	}
	if overlays := trendlymodels.OverlayClips(c); len(overlays) > 0 {
		rows := make([]map[string]any, 0, len(overlays))
		for _, o := range overlays {
			rows = append(rows, map[string]any{
				"startMs":    o.StartMs,
				"durationMs": o.DurationMs,
			})
		}
		out["overlays"] = rows
	}
	return out
}

func videoTrackClips(c *trendlymodels.Composition) []trendlymodels.CompositionClip {
	for _, t := range c.Tracks {
		if t.Kind == trendlymodels.CompositionTrackVideo {
			return t.Clips
		}
	}
	return nil
}
