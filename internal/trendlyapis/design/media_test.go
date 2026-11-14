package design

import (
	"strings"
	"testing"
)

const mediaFilm = `<!doctype html>
<html><body>
  <div id="root" data-composition-id="main" data-duration="6">
    <section id="s1" data-scene="hook" data-start="0" data-duration="3">
      <h1 id="hook-title" class="clip" data-start="0" data-duration="3">One</h1>
      <img id="hero" class="clip" data-media="image" data-media-intent="old idea"
           data-media-resolved="assets/old.jpg" src="assets/old.jpg" data-start="0" data-duration="3" />
    </section>
    <section id="s2" data-scene="end" data-start="3" data-duration="3">
      <h1 id="end-title" class="clip" data-start="3" data-duration="3">Two</h1>
    </section>
  </div>
  <script>gsap.timeline();</script>
</body></html>`

func TestAddingMediaToASceneDeclaresIt(t *testing.T) {
	out, note, err := applyMediaPlacement(mediaFilm, placeMediaRequest{
		SceneID: "s2", Type: "image", Intent: "a flat white on a wooden table",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `data-media="image"`) ||
		!strings.Contains(out, `data-media-intent="a flat white on a wooden table"`) {
		t.Fatalf("no declaration was written:\n%s", out)
	}
	// Timed to the scene it went into, or a photograph dropped into the
	// second scene is on screen over the first as well.
	if !strings.Contains(out, `data-start="3" data-duration="3"`) {
		t.Fatalf("the new element is not timed to its scene:\n%s", out)
	}
	// Inside that scene, so it moves with it.
	s2 := strings.Index(out, `id="s2"`)
	img := strings.Index(out, `id="image-1"`)
	if img < s2 {
		t.Fatalf("the element landed outside its scene")
	}
	if !strings.Contains(note, "end") {
		t.Fatalf("the revision note does not say where it went: %q", note)
	}
}

func TestAnAddedImageIsSizedToItsSlot(t *testing.T) {
	out, _, err := applyMediaPlacement(mediaFilm, placeMediaRequest{
		SceneID: "s2", Type: "image", Intent: "a flat white",
	})
	if err != nil {
		t.Fatal(err)
	}
	// A 4000px stock photograph at its natural size is a film with one
	// corner of a photograph in it.
	if !strings.Contains(out, "object-fit:cover") {
		t.Fatalf("the image was not sized:\n%s", out)
	}
}

func TestMusicIsAnAudioElement(t *testing.T) {
	out, _, err := applyMediaPlacement(mediaFilm, placeMediaRequest{
		SceneID: "s1", Type: "bgm", Intent: "upbeat lo-fi, no vocals",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `<audio id="bgm-1"`) {
		t.Fatalf("music was not added as audio:\n%s", out)
	}
}

func TestPlacingAFileTheProjectAlreadyHasIsImmediate(t *testing.T) {
	out, _, err := applyMediaPlacement(mediaFilm, placeMediaRequest{
		SceneID: "s2", Type: "image", AssetPath: "assets/shot.jpg",
	})
	if err != nil {
		t.Fatal(err)
	}
	// src AND resolved, so the next render does not fetch something else
	// over the top of the choice just made.
	if !strings.Contains(out, `src="assets/shot.jpg" data-media-resolved="assets/shot.jpg"`) {
		t.Fatalf("the file was not placed:\n%s", out)
	}
	if strings.Contains(out, `data-media="image" data-media-intent=""`) {
		t.Fatal("an empty declaration was left behind")
	}
}

func TestReplacingTheMediaOnAnElement(t *testing.T) {
	out, _, err := applyMediaPlacement(mediaFilm, placeMediaRequest{
		ElementID: "hero", AssetPath: "assets/new.jpg",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `src="assets/new.jpg"`) || strings.Contains(out, `src="assets/old.jpg"`) {
		t.Fatalf("the media did not change:\n%s", out)
	}
	// Nothing around it moved.
	if !strings.Contains(out, `<h1 id="hook-title" class="clip" data-start="0" data-duration="3">One</h1>`) {
		t.Fatal("something else in the scene changed")
	}
	if strings.Count(out, "<img") != 1 {
		t.Fatal("a second image was added instead of replacing the first")
	}
}

func TestReDeclaringAnElementAsksForItAgain(t *testing.T) {
	out, _, err := applyMediaPlacement(mediaFilm, placeMediaRequest{
		ElementID: "hero", Intent: "cold brew poured over ice",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `data-media-intent="cold brew poured over ice"`) {
		t.Fatalf("the words did not change:\n%s", out)
	}
	// Both markers cleared, or the resolver skips it and the film keeps
	// the old photograph.
	if !strings.Contains(out, `src=""`) || !strings.Contains(out, `data-media-resolved=""`) {
		t.Fatalf("the old file is still attached:\n%s", out)
	}
}

// The src is resolved relative to the project directory, so a link here
// would put a remote dependency into a film meant to render with no
// network.
func TestAFilmsMediaHasToBeAFileInTheProject(t *testing.T) {
	bad := map[string]string{
		"https://example.com/a.jpg": "a link",
		"//example.com/a.jpg":       "a protocol-relative link",
		"/etc/passwd":               "an absolute path",
		"../../other/a.jpg":         "a traversal",
		"assets/notes.txt":          "a file a film cannot play",
	}
	for p, why := range bad {
		if _, _, err := applyMediaPlacement(mediaFilm, placeMediaRequest{
			SceneID: "s1", Type: "image", AssetPath: p,
		}); err == nil {
			t.Fatalf("accepted %q (%s)", p, why)
		}
	}
}

func TestRefusedPlacements(t *testing.T) {
	cases := []struct {
		req  placeMediaRequest
		what string
	}{
		{placeMediaRequest{SceneID: "s1", Type: "hologram", Intent: "x"}, "a kind of media that does not exist"},
		{placeMediaRequest{SceneID: "s1", Type: "image"}, "neither words nor a file"},
		{placeMediaRequest{SceneID: "gone", Type: "image", Intent: "x"}, "a scene that is not there"},
		{placeMediaRequest{ElementID: "hook-title", AssetPath: "assets/a.jpg"}, "an element that does not hold media"},
		{placeMediaRequest{ElementID: "gone", AssetPath: "assets/a.jpg"}, "an element that is not there"},
		{placeMediaRequest{SceneID: "s1", Type: "image", Intent: strings.Repeat("x", 400)}, "a pasted document"},
	}
	for _, c := range cases {
		if _, _, err := applyMediaPlacement(mediaFilm, c.req); err == nil {
			t.Fatalf("accepted %s", c.what)
		}
	}
}

func TestAssetKind(t *testing.T) {
	for p, want := range map[string]string{
		"assets/a.JPG": "image", "assets/b.webp": "image", "assets/c.svg": "image",
		"assets/voice/d.wav": "audio", "assets/e.mp3": "audio",
		"assets/f.mp4": "video", "index.html": "", "assets/g.woff2": "",
	} {
		if got := assetKind(p); got != want {
			t.Fatalf("%s is %q, want %q", p, got, want)
		}
	}
}
