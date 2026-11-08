package trendlymodels

// Rendering a studio: what it costs, whether it can render at all, and where
// the status lives.
//
// ── Why the checks are here and not only in the worker ─────────────────────
// The worker would reject an unrenderable studio too — but by then the wallet
// has paid for an attempt that was never going to produce anything. So the same
// questions are asked before the charge, exactly as they are for a composition.
//
// ⚠️ THESE CHECKS MIRROR `studioToComposition` in
// shared-libs/design/studio-runtime.ts. That function is the one the worker
// actually executes; this is the pre-flight. They have to agree, so each rule
// below names the same limitation in the same terms. If one gains a rule, so
// does the other.
//
// The two limitations worth knowing:
//
//  1. A LAYERED SCENE CANNOT EXPORT YET. A composition clip is one full-frame
//     source with no concept of a slot, so a split screen is previewable and
//     not renderable. Rendering only the base layer would publish something
//     nobody approved, which is worse than refusing.
//  2. STYLISED TRANSITIONS RENDER AS DISSOLVES. The composition runtime knows
//     `cut` and `fade`. That is a note rather than a refusal — the output is
//     still the right length and the right cut points — but it has to be said,
//     or someone approves a whip pan and publishes a cross-fade.

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
)

// StudioRenderable reports whether a studio can render as authored.
//
// Returns false with a sentence for the user. `notes` are differences that do
// not block — things the output will do differently that the user should know
// before approving it.
func StudioRenderable(
	studio *Studio,
	scenes []StudioScene,
	media map[string]*Asset,
) (ok bool, problem string, notes []string) {
	if studio == nil {
		return false, "There is no studio to render.", nil
	}
	ordered := OrderedScenes(studio, scenes)
	if len(ordered) == 0 {
		return false, "There are no scenes to render yet.", nil
	}

	stylised := false
	for i, sc := range ordered {
		position := i + 1

		if len(sc.Layers) == 0 {
			return false, fmt.Sprintf(
				"Scene %d is empty. Put something in it or remove it.", position), nil
		}
		if len(sc.Layers) > 1 {
			return false, fmt.Sprintf(
				"Scene %d has %d layers. Layered scenes can be previewed but not exported yet — "+
					"flatten it to one layer to render.", position, len(sc.Layers)), nil
		}
		if EffectiveSceneMs(sc) <= 0 {
			return false, fmt.Sprintf("Scene %d has no length.", position), nil
		}
		if sc.Pending {
			return false, fmt.Sprintf(
				"Scene %d is still being made. Wait for it to land before rendering.", position), nil
		}

		layer := sc.Layers[0]
		switch layer.Kind {
		case "html":
			if layer.HTML == "" {
				return false, fmt.Sprintf("Scene %d has no design in it.", position), nil
			}
		case "media":
			a := media[layer.MediaID]
			if a == nil || a.URL == "" {
				return false, fmt.Sprintf(
					"Scene %d's media is missing. It may have been archived — replace it.",
					position), nil
			}
			if a.Kind == AssetKindAudio {
				return false, fmt.Sprintf(
					"Scene %d points at audio, which cannot be a picture.", position), nil
			}
		default:
			return false, fmt.Sprintf("Scene %d has nothing renderable in it.", position), nil
		}

		if i > 0 && sc.TransitionIn != nil &&
			sc.TransitionIn.Kind != TransitionCut && sc.TransitionIn.Kind != TransitionDissolve {
			stylised = true
		}
	}

	if stylised {
		notes = append(notes,
			"Wipes, slides and whips render as dissolves for now. The cuts land in the same places.")
	}
	return true, "", notes
}

// StudioRenderTokenCost prices a render.
//
// Deliberately the same shape as a composition's: a studio IS a composition by
// the time the worker sees it, and two different prices for the same work would
// be arbitrary. Scene count stands in for billable clips, since each scene is
// one intermediate render, and the decoded milliseconds are what the encode
// actually has to chew through.
func StudioRenderTokenCost(studio *Studio, scenes []StudioScene, media map[string]*Asset) int64 {
	m := MeasureStudio(studio, scenes)
	decoded := 0
	for _, sc := range m.Scenes {
		// A video layer is decoded for as long as it is on screen; a still and
		// an HTML scene are captured instead, which is cheaper per second but
		// not free — the composition pricing already accounts for that split,
		// so only real decoding is counted here.
		if len(sc.Layers) == 0 {
			continue
		}
		if a := media[sc.Layers[0].MediaID]; a != nil && a.Kind == AssetKindVideo {
			decoded += EffectiveSceneMs(sc)
		}
	}
	return CompositionRenderTokenCost(m.TotalMs, len(m.Scenes), decoded)
}

// ── Render status, on the studio document ──────────────────────────────────
//
// On the studio rather than in the renders subcollection, for the same reason
// the design and composition revisions carry their own: the app is already
// subscribed to the studio, so a status there needs no second listener. The
// subcollection keeps the HISTORY of renders; this is the current one.

// MarkStudioRenderQueued stamps a render as queued BEFORE it is dispatched, so
// the app shows the job the instant the request returns rather than sitting on
// a stale state until a worker picks it up.
func MarkStudioRenderQueued(ctx context.Context, brandID, contentID, studioID, jobID string) error {
	return UpdateStudio(ctx, brandID, contentID, studioID, []firestore.Update{
		{Path: "renderStatus", Value: DesignRenderQueued},
		{Path: "renderId", Value: jobID},
	})
}

func MarkStudioRenderFailed(ctx context.Context, brandID, contentID, studioID, reason string) error {
	return UpdateStudio(ctx, brandID, contentID, studioID, []firestore.Update{
		{Path: "renderStatus", Value: DesignRenderFailed},
		{Path: "renderError", Value: reason},
	})
}

// SetStudioRenderResult records a finished render.
//
// The URL goes on the studio AND on the cached content pointer, because the
// calendar card reads the pointer and would otherwise show the previous render
// until something else happened to refresh it.
func SetStudioRenderResult(ctx context.Context, brandID, contentID, studioID, url string) error {
	if err := UpdateStudio(ctx, brandID, contentID, studioID, []firestore.Update{
		{Path: "renderStatus", Value: DesignRenderDone},
		{Path: "renderUrl", Value: url},
		{Path: "renderError", Value: ""},
	}); err != nil {
		return err
	}

	studio, err := GetStudio(ctx, brandID, contentID, studioID)
	if err != nil || studio == nil {
		return err
	}
	scenes, err := ListStudioScenes(ctx, brandID, contentID, studioID)
	if err != nil {
		return err
	}
	m := MeasureStudio(studio, scenes)
	return SetContentStudioRef(ctx, brandID, contentID, &StudioRef{
		StudioID:   studioID,
		SceneCount: len(m.Scenes),
		DurationMs: m.TotalMs,
		RenderURL:  url,
		UpdatedAt:  time.Now().UnixMilli(),
	})
}

// StudioSceneHTMLRef is the synthetic revision id a scene's HTML is loaded by.
//
// Mirrors sceneHtmlRef in shared-libs/design/studio-runtime.ts. A studio scene
// carries its HTML on the layer rather than in a `designs` revision, and the
// renderer resolves design clips through an injected loader — so the id says
// where to look and the worker's loader understands the prefix.
func StudioSceneHTMLRef(studioID, sceneID string) string {
	return fmt.Sprintf("studio:%s:%s", studioID, sceneID)
}
