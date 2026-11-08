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
//  1. A LAYERED SCENE EXPORTS UNLESS IT CONTAINS VIDEO. A stack of pictures and
//     documents is a layout, and Chromium draws layouts, so the conversion
//     composes it into one document. Footage is decoded by FFmpeg and never by
//     the browser, so a stack containing video still cannot export — and the
//     refusal names the layer rather than the stack.
//  2. ONLY A WHIP IS NOT ITSELF. Each transition is drawn by the xfade of the
//     same name; a whip pan needs directional motion blur, which xfade has
//     nothing for, so it renders as the slide and says so.
//
// ⚠️ THIS FILE IS THE REASON TO BE CAREFUL. When the TypeScript side narrowed
// both of those refusals, this one did not — and because THIS is what the
// preflight and the render gate call, the backend kept refusing a layered scene
// the worker would happily have rendered. The deploy was green and every test
// passed; it was caught by rendering a split screen on dev and reading the 400.
// Two implementations of one rule stay in step only while somebody remembers
// they exist.

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
		if EffectiveSceneMs(sc) <= 0 {
			return false, fmt.Sprintf("Scene %d has no length.", position), nil
		}
		if sc.Pending {
			return false, fmt.Sprintf(
				"Scene %d is still being made. Wait for it to land before rendering.", position), nil
		}

		// Every layer, not just the first. A stack is composed into one
		// document, so each layer in it has to be drawable — and a stack is
		// refused for the LAYER that cannot be drawn rather than for being a
		// stack, which is the difference between "flatten it" and "that one is
		// a video".
		stack := len(sc.Layers) > 1
		for li, layer := range sc.Layers {
			where := fmt.Sprintf("Scene %d", position)
			if stack {
				where = fmt.Sprintf("Scene %d's layer %d", position, li+1)
			}
			switch layer.Kind {
			case "html":
				if layer.HTML == "" {
					return false, where + " has no design in it.", nil
				}
			case "media":
				a := media[layer.MediaID]
				if a == nil || a.URL == "" {
					return false, where +
						"'s media is missing. It may have been archived — replace it.", nil
				}
				if a.Kind == AssetKindAudio {
					return false, where + " points at audio, which cannot be a picture.", nil
				}
				if stack && a.Kind == AssetKindVideo {
					// The one thing a composed document cannot hold.
					return false, fmt.Sprintf(
						"Scene %d can't be exported yet because layer %d is video. A split "+
							"screen of stills or type over a still does export — it's video "+
							"inside a stack that doesn't.", position, li+1), nil
				}
			default:
				return false, where + " has nothing renderable in it.", nil
			}
		}

		// Only a whip. The other six are drawn by the xfade of the same name.
		if i > 0 && sc.TransitionIn != nil && sc.TransitionIn.Kind == TransitionWhip {
			stylised = true
		}
	}

	if stylised {
		notes = append(notes,
			"A whip renders as a fast slide — the blur it needs isn't something the encoder can draw yet.")
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
		// Cleared here and not only when a worker starts. A previous failure's
		// message outlived it: the studio read `renderStatus: queued` with
		// "We couldn't start the render. Please try again." still sitting
		// beside it, so a render that was queued and working looked like one
		// that had just failed. Whoever queues a new attempt owns retiring the
		// last one's excuse — the worker may not pick it up for a minute.
		{Path: "renderError", Value: ""},
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
