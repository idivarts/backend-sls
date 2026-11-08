package trendlymodels

import "testing"

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

// The refusal that matters most. A layered scene cannot export yet, and
// rendering only its base layer would publish something nobody approved —
// which is worse than refusing, because nobody would know it happened.
func TestALayeredSceneIsRefusedBeforeCharging(t *testing.T) {
	s, scenes, media := renderableStudio()
	scenes[0].Layers = append(scenes[0].Layers,
		StudioLayer{ID: "l2", Kind: "media", Slot: SlotPipBR, MediaID: "m1"})

	ok, problem, _ := StudioRenderable(s, scenes, media)
	if ok {
		t.Fatal("a layered scene should refuse to render")
	}
	if problem == "" {
		t.Fatal("a refusal has to carry a sentence the user can act on")
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
