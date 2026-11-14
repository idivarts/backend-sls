package design

import (
	"strings"
	"testing"
)

const dropDoc = `<div id="root" data-composition-id="reel" data-duration="10">
  <section id="s1" class="clip" data-scene="hook" data-start="0" data-duration="4"></section>
  <section id="s2" class="clip" data-scene="end" data-start="4" data-duration="6"></section>
</div>`

// "Drop a file on a track and it is in the film at that moment."
func TestDropAFileOnARowAtAMoment(t *testing.T) {
	out, note, err := applyMediaPlacement(dropDoc, placeMediaRequest{
		Type: "image", AssetPath: "assets/pour.jpg",
		StartMs: intp(6000), TrackIndex: intp(1),
	})
	if err != nil {
		t.Fatalf("dropping a file: %v", err)
	}
	if !strings.Contains(out, `data-start="6"`) || !strings.Contains(out, `data-track-index="1"`) {
		t.Fatalf("it did not land where it was dropped:\n%s", out)
	}
	if !strings.Contains(out, `src="assets/pour.jpg"`) {
		t.Fatalf("the file is not in it:\n%s", out)
	}
	// ⚠️ At the ROOT, not inside a scene. A scene would clip anything timed
	// outside its own window, and the film would render without it.
	for _, id := range []string{`id="s1"`, `id="s2"`} {
		at := strings.Index(out, id)
		end := strings.Index(out[at:], "</section>")
		if strings.Contains(out[at:at+end], "assets/pour.jpg") {
			t.Fatalf("it was put inside %s, which ends before 6s:\n%s", id, out)
		}
	}
	// And it says when, because that is what the person chose.
	if !strings.Contains(note, "0:06") {
		t.Fatalf("the revisions list would say %q", note)
	}
}

// "Ask for a sound at 0:06 and it arrives there." Declared rather than
// placed: there is no file yet, so the next preview fetches one.
func TestAskForASoundAtAMoment(t *testing.T) {
	out, _, err := applyMediaPlacement(dropDoc, placeMediaRequest{
		Type: "sfx", Intent: "a soft shutter click", StartMs: intp(6000),
	})
	if err != nil {
		t.Fatalf("asking for a sound: %v", err)
	}
	if !strings.Contains(out, `data-media="sfx"`) ||
		!strings.Contains(out, `data-media-intent="a soft shutter click"`) {
		t.Fatalf("the declaration is not there for the worker to resolve:\n%s", out)
	}
	if !strings.Contains(out, `data-start="6"`) {
		t.Fatalf("it did not arrive at 0:06:\n%s", out)
	}
	// On the row its kind belongs on, since nothing said otherwise.
	if !strings.Contains(out, `data-track-index="3"`) {
		t.Fatalf("a sound effect should default to the sound row:\n%s", out)
	}
	// Nothing resolved, so the worker will fetch it.
	if strings.Contains(out, "data-media-resolved") {
		t.Fatalf("it was marked resolved with no file:\n%s", out)
	}
}

// A drop with no explicit length is visible but not the rest of the film.
func TestDropDefaultsToAReadableLength(t *testing.T) {
	out, _, err := applyMediaPlacement(dropDoc, placeMediaRequest{
		Type: "image", AssetPath: "assets/a.jpg", StartMs: intp(1000),
	})
	if err != nil {
		t.Fatalf("dropping: %v", err)
	}
	if !strings.Contains(out, `data-duration="3"`) {
		t.Fatalf("want the three-second default:\n%s", out)
	}
}

// Never longer than the film it is in: a bar wider than its row pushes the
// whole timeline out of alignment.
func TestDropIsTrimmedToTheFilm(t *testing.T) {
	out, _, err := applyMediaPlacement(dropDoc, placeMediaRequest{
		Type: "bgm", Intent: "lo-fi", StartMs: intp(9000), DurationMs: intp(8000),
	})
	if err != nil {
		t.Fatalf("dropping music near the end: %v", err)
	}
	if !strings.Contains(out, `data-start="9" data-duration="1"`) {
		t.Fatalf("it runs past the end of the film:\n%s", out)
	}
}

// A drop past the end of the film is a drop at the end of the film.
func TestDropPastTheEndIsClamped(t *testing.T) {
	out, _, err := applyMediaPlacement(dropDoc, placeMediaRequest{
		Type: "image", AssetPath: "assets/a.jpg", StartMs: intp(99000),
	})
	if err != nil {
		t.Fatalf("dropping past the end: %v", err)
	}
	if !strings.Contains(out, `data-start="9.9"`) {
		t.Fatalf("it was not clamped to the film:\n%s", out)
	}
}

func TestDropRejectsNonsense(t *testing.T) {
	for _, req := range []placeMediaRequest{
		{Type: "image", AssetPath: "assets/a.jpg", StartMs: intp(0), DurationMs: intp(0)},
		{Type: "image", AssetPath: "assets/a.jpg", StartMs: intp(0), TrackIndex: intp(900)},
		{Type: "biscuit", AssetPath: "assets/a.jpg", StartMs: intp(0)},
		{Type: "image", StartMs: intp(0)}, // no file and no words
	} {
		if _, _, err := applyMediaPlacement(dropDoc, req); err == nil {
			t.Fatalf("accepted %+v", req)
		}
	}
}

// Placing into a scene still works exactly as it did — a photograph that
// belongs to a shot should move with that shot.
func TestPlacingIntoASceneIsUnchanged(t *testing.T) {
	out, _, err := applyMediaPlacement(dropDoc, placeMediaRequest{
		Type: "image", AssetPath: "assets/a.jpg", SceneID: "end",
	})
	if err != nil {
		t.Fatalf("placing in a scene: %v", err)
	}
	at := strings.Index(out, `id="s2"`)
	end := at + strings.Index(out[at:], "</section>")
	if !strings.Contains(out[at:end], "assets/a.jpg") {
		t.Fatalf("it did not go inside the scene it was meant for:\n%s", out)
	}
	if !strings.Contains(out[at:end], `data-start="4" data-duration="6"`) {
		t.Fatalf("it did not inherit the scene's window:\n%s", out[at:end])
	}
}
