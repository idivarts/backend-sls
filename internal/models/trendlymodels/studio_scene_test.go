package trendlymodels

import (
	"testing"

	"cloud.google.com/go/firestore"
)

func scene(id string, ms int) StudioScene {
	return StudioScene{ID: id, DurationMs: ms, PrimaryKind: SceneImage}
}

func withTransition(sc StudioScene, kind string, ms int) StudioScene {
	sc.TransitionIn = &Transition{Kind: kind, DurationMs: ms}
	return sc
}

// The rule that is easiest to get backwards and most expensive when you do: a
// transition is an OVERLAP, so adding one makes the timeline SHORTER. A render
// built on the opposite assumption drifts longer with every dissolve and its
// audio stops matching its picture.
func TestTransitionsShortenTheTimeline(t *testing.T) {
	studio := &Studio{SceneOrder: []string{"a", "b", "c"}}
	scenes := []StudioScene{
		scene("a", 3000),
		withTransition(scene("b", 3000), TransitionDissolve, 400),
		withTransition(scene("c", 3000), TransitionDissolve, 400),
	}

	m := MeasureStudio(studio, scenes)

	// 9000ms of scenes, minus two 400ms overlaps.
	if m.TotalMs != 8200 {
		t.Errorf("total = %dms, want 8200ms (9000 minus two 400ms overlaps)", m.TotalMs)
	}
	if m.Starts["a"] != 0 {
		t.Errorf("a starts at %d, want 0", m.Starts["a"])
	}
	if m.Starts["b"] != 2600 {
		t.Errorf("b starts at %d, want 2600 (3000 minus its 400ms overlap)", m.Starts["b"])
	}
	if m.Starts["c"] != 5200 {
		t.Errorf("c starts at %d, want 5200", m.Starts["c"])
	}
}

// A cut costs nothing, which is why it is the default.
func TestCutsCostNothing(t *testing.T) {
	studio := &Studio{SceneOrder: []string{"a", "b"}}
	scenes := []StudioScene{scene("a", 2000), withTransition(scene("b", 2000), TransitionCut, 500)}
	if m := MeasureStudio(studio, scenes); m.TotalMs != 4000 {
		t.Errorf("total = %dms, want 4000ms — a cut has no overlap", m.TotalMs)
	}
}

// A transition longer than the scene it is dissolving out of has nothing left
// to dissolve. Without the clamp, a planner asking for a cinematic cross-fade
// between two quick beats collapses the timeline.
func TestOverlapIsClampedToHalfTheShorterNeighbour(t *testing.T) {
	prev := scene("a", 600)
	next := withTransition(scene("b", 5000), TransitionDissolve, 1000)

	got := TransitionOverlapMs(prev, next)
	if got != 300 {
		t.Errorf("overlap = %dms, want 300ms (half of the 600ms neighbour)", got)
	}
}

// fitToNarration is the whole answer to "the voice overruns the picture": the
// scene stretches to hold the line instead.
func TestFitToNarrationStretchesTheScene(t *testing.T) {
	// 42 characters at 14/sec is 3000ms, plus the 300ms tail.
	line := "Sunday night is when the panic sets in now."
	if len(line) != 43 {
		t.Fatalf("fixture drifted: the line is %d characters", len(line))
	}

	fitted := StudioScene{ID: "a", DurationMs: 1000,
		Narration: &SceneNarration{Line: line, FitToNarration: true}}
	if got := EffectiveSceneMs(fitted); got <= 1000 {
		t.Errorf("effective = %dms, want it stretched past the stored 1000ms", got)
	}

	// Without the flag the stored duration stands — and the over-long line is
	// a validation failure elsewhere, not a silent overrun here.
	notFitted := StudioScene{ID: "a", DurationMs: 1000,
		Narration: &SceneNarration{Line: line}}
	if got := EffectiveSceneMs(notFitted); got != 1000 {
		t.Errorf("effective = %dms, want the stored 1000ms left alone", got)
	}
}

func TestNarrationMsGrowsWithTheLine(t *testing.T) {
	if NarrationMs("") != 0 {
		t.Error("an empty line takes no time")
	}
	short := NarrationMs("Hello there.")
	long := NarrationMs("Hello there, this is a considerably longer line to say out loud.")
	if !(long > short) {
		t.Errorf("a longer line should take longer: %d vs %d", long, short)
	}
	if short <= NarrationTailMs {
		t.Errorf("a line should cost more than the tail alone: %d", short)
	}
}

// sceneOrder is the authority. A scene missing from it is a half-landed write,
// and appending it is better than hiding it: the user can see and delete a
// stray scene, where a hidden one is a document nobody can reach.
func TestAStraySceneIsAppendedNotHidden(t *testing.T) {
	studio := &Studio{SceneOrder: []string{"b"}}
	scenes := []StudioScene{scene("a", 1000), scene("b", 1000)}

	ordered := OrderedScenes(studio, scenes)
	if len(ordered) != 2 {
		t.Fatalf("got %d scenes, want both", len(ordered))
	}
	if ordered[0].ID != "b" || ordered[1].ID != "a" {
		t.Errorf("order = %s,%s — want the ordered one first and the stray appended",
			ordered[0].ID, ordered[1].ID)
	}
}

// The same media placed twice in one scene must count ONCE, or clearing the
// scene leaves the reference count stuck above zero and the media can never be
// archived.
func TestSceneMediaIDsDedupe(t *testing.T) {
	sc := &StudioScene{Layers: []StudioLayer{
		{ID: "l1", Kind: "media", MediaID: "m1"},
		{ID: "l2", Kind: "media", MediaID: "m1"},
		{ID: "l3", Kind: "html"},
		{ID: "l4", Kind: "media", MediaID: "m2"},
	}}
	got := SceneMediaIDs(sc)
	if len(got) != 2 {
		t.Fatalf("got %v, want two ids", got)
	}
}

// Swapping one layer of a two-layer scene must not drop the untouched layer's
// media to zero for an instant — an archive sweep landing in that instant
// deletes something that is on screen.
func TestLayerSwapOnlyMovesWhatChanged(t *testing.T) {
	added, removed := diffStrings([]string{"m1", "m2"}, []string{"m1", "m3"})
	if len(added) != 1 || added[0] != "m3" {
		t.Errorf("added = %v, want [m3]", added)
	}
	if len(removed) != 1 || removed[0] != "m2" {
		t.Errorf("removed = %v, want [m2]", removed)
	}
}

// UpdateStudioScene must refuse the fields that have to move reference counts
// with them, rather than trusting every future caller to remember.
func TestUpdateSceneRefusesLayerWrites(t *testing.T) {
	for _, path := range []string{"layers", "mediaIds"} {
		// The refusal happens before any Firestore call, so a nil context is
		// fine and the test needs no emulator.
		if err := UpdateStudioScene(nil, "b", "c", "s", "sc",
			[]firestore.Update{{Path: path, Value: 1}}); err == nil {
			t.Errorf("should refuse a %q write", path)
		}
	}
}
