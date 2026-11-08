package trendlymodels

import (
	"strings"
	"testing"
)

func renderScene(id string, layers []StudioLayer) StudioScene {
	return StudioScene{ID: id, DurationMs: 3000, PrimaryKind: SceneImage, Layers: layers}
}

func mediaLayer(mediaID string) []StudioLayer {
	return []StudioLayer{{ID: "l1", Kind: "media", Slot: SlotFull, MediaID: mediaID}}
}

func renderableStudio() (*Studio, []StudioScene, map[string]*Asset) {
	s := &Studio{ID: "st", Width: 1080, Height: 1920, FPS: 30, SceneOrder: []string{"a", "b"}}
	scenes := []StudioScene{
		renderScene("a", mediaLayer("m1")),
		renderScene("b", mediaLayer("m1")),
	}
	media := map[string]*Asset{
		"m1": {ID: "m1", Kind: AssetKindImage, URL: "https://cdn/x.jpg"},
	}
	return s, scenes, media
}

func TestARenderableStudioPasses(t *testing.T) {
	s, scenes, media := renderableStudio()
	ok, problem, _ := StudioRenderable(s, scenes, media)
	if !ok {
		t.Fatalf("should be renderable, got: %s", problem)
	}
}

// This used to assert that a layered scene was refused outright. It no longer
// is: a stack of stills is a LAYOUT, Chromium draws layouts, and the
// conversion composes it into one document. The refusal narrowed to the case
// that genuinely cannot work — see TestAStackWithVideoIsRefusedByItsLayer —
// and this test now asserts the narrowing rather than the old blanket rule.
func TestAStillOverAStillExportsRatherThanRefusing(t *testing.T) {
	s, scenes, media := renderableStudio()
	scenes[0].Layers = append(scenes[0].Layers,
		StudioLayer{ID: "l2", Kind: "media", Slot: SlotPipBR, MediaID: "m1"})

	ok, problem, _ := StudioRenderable(s, scenes, media)
	if !ok {
		t.Fatalf("a still inset over a still should export, got %q", problem)
	}
}

// A scene whose media has been archived out from under it would otherwise
// render as a gap.
func TestMissingMediaIsRefused(t *testing.T) {
	s, scenes, _ := renderableStudio()
	ok, problem, _ := StudioRenderable(s, scenes, map[string]*Asset{})
	if ok {
		t.Fatal("a scene pointing at media that is gone should refuse")
	}
	if problem == "" {
		t.Fatal("expected a sentence about the missing media")
	}
}

// A generation still in flight is not a render-time failure, it is a wait.
// Charging for a render of a scene that is about to change is a wasted charge.
func TestAPendingSceneIsRefused(t *testing.T) {
	s, scenes, media := renderableStudio()
	scenes[1].Pending = true
	if ok, _, _ := StudioRenderable(s, scenes, media); ok {
		t.Fatal("a studio with a scene still being made should not render yet")
	}
}

func TestAnEmptyStudioIsRefused(t *testing.T) {
	if ok, problem, _ := StudioRenderable(&Studio{ID: "st"}, nil, nil); ok || problem == "" {
		t.Fatal("a studio with no scenes should refuse, with a reason")
	}
}

// Audio in a picture slot is a client bug, and rendering it would produce a
// black frame for the scene's whole length.
func TestAudioInAPictureSlotIsRefused(t *testing.T) {
	s, scenes, _ := renderableStudio()
	media := map[string]*Asset{"m1": {ID: "m1", Kind: AssetKindAudio, URL: "https://cdn/x.mp3"}}
	if ok, _, _ := StudioRenderable(s, scenes, media); ok {
		t.Fatal("audio cannot be a picture")
	}
}

// A stylised transition does NOT block — the cuts still land in the right
// places — but it has to be said, or someone approves a whip pan and publishes
// a dissolve.
func TestStylisedTransitionsAreANoteNotARefusal(t *testing.T) {
	s, scenes, media := renderableStudio()
	scenes[1].TransitionIn = &Transition{Kind: TransitionWhip, DurationMs: 200}

	ok, problem, notes := StudioRenderable(s, scenes, media)
	if !ok {
		t.Fatalf("a stylised transition should not block the render, got: %s", problem)
	}
	if len(notes) == 0 {
		t.Fatal("it should be reported as a note")
	}
}

// A dissolve renders exactly, so it needs no note — a note on every render
// would train people to ignore them.
func TestADissolveNeedsNoNote(t *testing.T) {
	s, scenes, media := renderableStudio()
	scenes[1].TransitionIn = &Transition{Kind: TransitionDissolve, DurationMs: 400}

	if _, _, notes := StudioRenderable(s, scenes, media); len(notes) != 0 {
		t.Errorf("a dissolve renders faithfully and needs no note, got %v", notes)
	}
}

// The synthetic ref has to match the TypeScript side exactly, or the worker
// cannot resolve a scene's HTML.
func TestSceneHTMLRefMatchesTheSharedFormat(t *testing.T) {
	if got := StudioSceneHTMLRef("st1", "sc9"); got != "studio:st1:sc9" {
		t.Errorf("ref = %q, want studio:st1:sc9", got)
	}
}

// ── The mirror, and why it has its own tests ───────────────────────────────
//
// StudioRenderable is a SECOND implementation of the rules in
// shared-libs/design/studio-runtime.ts, and the file says so at the top. When
// the TypeScript side narrowed its refusals, this one did not — and because
// THIS is what the preflight and the render gate call, the backend went on
// refusing a layered scene the worker would have rendered happily. Every test
// passed and the deploy was green; it was caught by rendering a split screen on
// dev and reading the 400.
//
// These lock the narrowed rules in place on this side.

func layered(layers ...StudioLayer) []StudioScene {
	return []StudioScene{{ID: "a", DurationMs: 3000, Layers: layers}}
}

func htmlLayer(id string) StudioLayer {
	return StudioLayer{ID: id, Kind: "html", Slot: "full", HTML: "<div>x</div>"}
}

func TestASplitScreenOfStillsCanExport(t *testing.T) {
	// It used to be refused outright with "flatten it to one layer to render".
	st := &Studio{SceneOrder: []string{"a"}, Width: 1080, Height: 1920, FPS: 30}
	ok, problem, _ := StudioRenderable(st, layered(htmlLayer("l1"), htmlLayer("l2")), nil)
	if !ok {
		t.Fatalf("a two-layer stack of documents should export, got %q", problem)
	}
}

func TestAStackWithVideoIsRefusedByItsLayer(t *testing.T) {
	// Footage is decoded by FFmpeg and never by Chromium, so it cannot go in a
	// composed document. The refusal names the layer, not the stack.
	st := &Studio{SceneOrder: []string{"a"}, Width: 1080, Height: 1920, FPS: 30}
	media := map[string]*Asset{"v": {URL: "https://cdn/x.mp4", Kind: AssetKindVideo}}
	ok, problem, _ := StudioRenderable(st,
		layered(htmlLayer("l1"), StudioLayer{ID: "l2", Kind: "media", MediaID: "v"}), media)
	if ok {
		t.Fatal("a stack containing video cannot export")
	}
	if !strings.Contains(problem, "layer 2 is video") {
		t.Errorf("the refusal should name the layer, got %q", problem)
	}
}

func TestAStackIsRefusedWhenALAYERSMediaIsGone(t *testing.T) {
	// The second layer, specifically — the check used to look at Layers[0] only,
	// so anything wrong with a layer behind the first was invisible to it.
	st := &Studio{SceneOrder: []string{"a"}, Width: 1080, Height: 1920, FPS: 30}
	ok, problem, _ := StudioRenderable(st,
		layered(htmlLayer("l1"), StudioLayer{ID: "l2", Kind: "media", MediaID: "gone"}), nil)
	if ok {
		t.Fatal("a stack whose media is missing cannot export")
	}
	if !strings.Contains(problem, "layer 2") {
		t.Errorf("the refusal should name the layer, got %q", problem)
	}
}

func TestOnlyAWhipIsStillNoted(t *testing.T) {
	// The other six are drawn by the xfade of the same name now. A note about
	// all of them would be telling the user something untrue.
	st := &Studio{SceneOrder: []string{"a", "b"}, Width: 1080, Height: 1920, FPS: 30}
	scene := func(id, kind string) StudioScene {
		sc := StudioScene{ID: id, DurationMs: 3000, Layers: []StudioLayer{htmlLayer("l1")}}
		if kind != "" {
			sc.TransitionIn = &Transition{Kind: kind, DurationMs: 400}
		}
		return sc
	}
	for _, kind := range []string{TransitionDissolve, TransitionWipe, TransitionSlide, TransitionDip} {
		_, _, notes := StudioRenderable(st, []StudioScene{scene("a", ""), scene("b", kind)}, nil)
		if len(notes) != 0 {
			t.Errorf("%s renders as itself and needs no note, got %v", kind, notes)
		}
	}
	_, _, notes := StudioRenderable(st, []StudioScene{scene("a", ""), scene("b", TransitionWhip)}, nil)
	if len(notes) != 1 || !strings.Contains(notes[0], "fast slide") {
		t.Errorf("a whip should still be noted, got %v", notes)
	}
}
