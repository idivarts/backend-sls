package design

import (
	"strings"
	"testing"
)

// A film with music over the whole thing, a voiceover and a photograph
// inside the first scene. The shape the phase's own test names.
const clipDoc = `<div id="root" data-composition-id="reel" data-duration="6">
  <section id="s1" class="clip" data-scene="hook" data-start="0" data-duration="3">
    <h1 id="h1" data-start="0" data-duration="3">Cold brew</h1>
    <img id="shot" class="clip" data-media="image" src="a.jpg" data-media-resolved="a.jpg"
         data-track-index="1" data-start="0" data-duration="3" />
  </section>
  <section id="s2" class="clip" data-scene="end" data-start="3" data-duration="3"></section>
  <audio id="score" class="clip" data-media="bgm" src="bgm.wav" data-media-resolved="bgm.wav"
         data-track-index="3" data-start="0" data-duration="6"></audio>
</div>`

func intp(n int) *int { return &n }

// The phase's own test, in one function: "Drag the music two seconds later
// and the render has it two seconds later, with the scenes untouched."
func TestClipRetimeMovesOnlyTheClip(t *testing.T) {
	out, filmMs, err := applyClipEdit(clipDoc, structureRequest{
		Op: clipOpRetime, ClipID: "score", StartMs: intp(2000),
	})
	if err != nil {
		t.Fatalf("moving the music: %v", err)
	}
	if !strings.Contains(out, `id="score"`) || !strings.Contains(out, `data-start="2"`) {
		t.Fatalf("the music did not move:\n%s", out)
	}
	// The scenes are where they were. This is the assertion the whole
	// separate code path exists for: a scene op would have relaid them.
	if !strings.Contains(out, `data-scene="hook" data-start="0" data-duration="3"`) {
		t.Fatalf("the first scene moved:\n%s", out)
	}
	if !strings.Contains(out, `data-scene="end" data-start="3" data-duration="3"`) {
		t.Fatalf("the second scene moved:\n%s", out)
	}
	if filmMs != 6000 {
		t.Fatalf("a clip op must not change the film's length: got %d", filmMs)
	}
}

// Trimming keeps the start, and moving keeps the length. A request that
// says one thing must not reset the other to zero.
func TestClipRetimeKeepsWhatItWasNotTold(t *testing.T) {
	out, _, err := applyClipEdit(clipDoc, structureRequest{
		Op: clipOpRetime, ClipID: "score", DurationMs: intp(4000),
	})
	if err != nil {
		t.Fatalf("trimming: %v", err)
	}
	if !strings.Contains(out, `data-start="0" data-duration="4"`) {
		t.Fatalf("a trim should keep the start:\n%s", out)
	}
}

// ⚠️ The trap this module exists for.
//
// A photograph appended inside the first scene, dragged to the second,
// would be CLIPPED by the scene holding it: the film renders, nothing
// errors, and the photograph is simply not in it.
func TestClipDraggedOutOfItsSceneIsLiftedToTheRoot(t *testing.T) {
	out, _, err := applyClipEdit(clipDoc, structureRequest{
		Op: clipOpRetime, ClipID: "shot", StartMs: intp(3500),
	})
	if err != nil {
		t.Fatalf("dragging the photograph: %v", err)
	}
	if !strings.Contains(out, `id="shot"`) {
		t.Fatalf("the photograph vanished:\n%s", out)
	}
	if !strings.Contains(out, `data-start="3.5"`) {
		t.Fatalf("it did not take its new time with it:\n%s", out)
	}
	// It is no longer inside the first scene — which is the point, because
	// the first scene ends at 3s.
	scene := out[strings.Index(out, `id="s1"`):strings.Index(out, `id="s2"`)]
	if strings.Contains(scene, `id="shot"`) {
		t.Fatalf("it is still trapped in the scene that would clip it:\n%s", scene)
	}
	// And the scene it left is still a well-formed scene with its headline.
	if !strings.Contains(scene, `id="h1"`) {
		t.Fatalf("lifting it took the headline with it:\n%s", scene)
	}
}

// Moving a clip WITHIN its scene's window leaves the markup alone. Lifting
// every drag out of its scene would detach a photograph from the scene it
// belongs to the first time someone nudged it.
func TestClipMovedInsideItsSceneStaysThere(t *testing.T) {
	out, _, err := applyClipEdit(clipDoc, structureRequest{
		Op: clipOpRetime, ClipID: "shot", StartMs: intp(500), DurationMs: intp(2000),
	})
	if err != nil {
		t.Fatalf("nudging the photograph: %v", err)
	}
	scene := out[strings.Index(out, `id="s1"`):strings.Index(out, `id="s2"`)]
	if !strings.Contains(scene, `id="shot"`) {
		t.Fatalf("it was lifted out for no reason:\n%s", scene)
	}
	if !strings.Contains(scene, `data-start="0.5" data-duration="2"`) {
		t.Fatalf("it did not move:\n%s", scene)
	}
}

// A scene is not a clip. Dragging one here would overlap the scene after
// it and the film would play two at once.
func TestClipOpsRefuseAScene(t *testing.T) {
	if _, _, err := applyClipEdit(clipDoc, structureRequest{
		Op: clipOpRetime, ClipID: "s1", StartMs: intp(1000),
	}); err == nil {
		t.Fatal("a scene was retimed as if it were a clip")
	}
	// And the root is the film itself: setting its start would move every
	// frame at once.
	if _, _, err := applyClipEdit(clipDoc, structureRequest{
		Op: clipOpRetime, ClipID: "root", StartMs: intp(1000),
	}); err == nil {
		t.Fatal("the film itself was retimed as a clip")
	}
}

func TestClipTrackMovesItToAnotherRow(t *testing.T) {
	out, _, err := applyClipEdit(clipDoc, structureRequest{
		Op: clipOpTrack, ClipID: "score", TrackIndex: intp(2),
	})
	if err != nil {
		t.Fatalf("changing the row: %v", err)
	}
	if !strings.Contains(out, `data-track-index="2"`) {
		t.Fatalf("it is on the same row:\n%s", out)
	}
	// A row change moves nothing in time, so it must not lift anything.
	if !strings.Contains(out, `data-start="0" data-duration="6"`) {
		t.Fatalf("a row change retimed it:\n%s", out)
	}
	if _, _, err := applyClipEdit(clipDoc, structureRequest{
		Op: clipOpTrack, ClipID: "score", TrackIndex: intp(900),
	}); err == nil {
		t.Fatal("row 900 was accepted, which would draw 900 empty rows")
	}
}

func TestClipDeleteRemovesOnlyThatClip(t *testing.T) {
	out, _, err := applyClipEdit(clipDoc, structureRequest{Op: clipOpDelete, ClipID: "score"})
	if err != nil {
		t.Fatalf("deleting: %v", err)
	}
	if strings.Contains(out, `id="score"`) {
		t.Fatalf("it is still there:\n%s", out)
	}
	for _, kept := range []string{`id="s1"`, `id="s2"`, `id="shot"`, `id="h1"`} {
		if !strings.Contains(out, kept) {
			t.Fatalf("deleting the music took %s with it:\n%s", kept, out)
		}
	}
}

// Dragging is imprecise; a drag that ends a little past the end of the film
// means the end of the film, not an error dialog.
func TestClipRetimeClampsToTheFilm(t *testing.T) {
	out, _, err := applyClipEdit(clipDoc, structureRequest{
		Op: clipOpRetime, ClipID: "score", StartMs: intp(99000),
	})
	if err != nil {
		t.Fatalf("dragging past the end: %v", err)
	}
	if !strings.Contains(out, `data-start="5.9"`) {
		t.Fatalf("it was not clamped to the film: \n%s", out)
	}
	// Negative means the start.
	out2, _, err := applyClipEdit(clipDoc, structureRequest{
		Op: clipOpRetime, ClipID: "score", StartMs: intp(-4000),
	})
	if err != nil {
		t.Fatalf("dragging before the start: %v", err)
	}
	if !strings.Contains(out2, `data-start="0"`) {
		t.Fatalf("a negative start was not clamped:\n%s", out2)
	}
	// A clip with no length is a row the renderer draws nothing of.
	if _, _, err := applyClipEdit(clipDoc, structureRequest{
		Op: clipOpRetime, ClipID: "score", DurationMs: intp(0),
	}); err == nil {
		t.Fatal("a zero-length clip was accepted")
	}
}

// A stale data-end contradicting the duration we just wrote is a film that
// plays to the old window in whichever reader prefers data-end.
func TestClipRetimeRewritesAStaleDataEnd(t *testing.T) {
	const doc = `<div id="root" data-composition-id="reel" data-duration="6">
  <audio id="score" class="clip" src="b.wav" data-start="0" data-duration="6" data-end="6"></audio>
</div>`
	out, _, err := applyClipEdit(doc, structureRequest{
		Op: clipOpRetime, ClipID: "score", StartMs: intp(1000), DurationMs: intp(2000),
	})
	if err != nil {
		t.Fatalf("retiming: %v", err)
	}
	if !strings.Contains(out, `data-end="3"`) {
		t.Fatalf("data-end still says the old window:\n%s", out)
	}
}

func TestClipOpsRejectNonsense(t *testing.T) {
	for _, req := range []structureRequest{
		{Op: clipOpRetime}, // no clip
		{Op: clipOpRetime, ClipID: "nope", StartMs: intp(1000)}, // gone from the film
		{Op: clipOpRetime, ClipID: "score"},                     // says nothing to change
		{Op: clipOpTrack, ClipID: "score"},                      // no row
		{Op: "clip-explode", ClipID: "score"},                   // not a thing
	} {
		if _, _, err := applyClipEdit(clipDoc, req); err == nil {
			t.Fatalf("accepted %+v", req)
		}
	}
}

// The ops routed through the structure endpoint reach the clip path, and
// the scene ops are untouched by its existence.
func TestStructureRoutesClipOpsAway(t *testing.T) {
	out, _, err := applyStructureEdit(clipDoc, structureRequest{
		Op: clipOpRetime, ClipID: "score", StartMs: intp(2000),
	})
	if err != nil {
		t.Fatalf("routing: %v", err)
	}
	if !strings.Contains(out, `data-scene="hook" data-start="0" data-duration="3"`) {
		t.Fatalf("a clip op relaid the scenes:\n%s", out)
	}
	if note := structureNote(structureRequest{Op: clipOpRetime, ClipID: "score", StartMs: intp(2000)}); note != "Moved score" {
		t.Fatalf("the revisions list would say %q", note)
	}
}
