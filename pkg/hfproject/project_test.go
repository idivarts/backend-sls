package hfproject

import (
	"strings"
	"testing"
)

func idx(extra string) File {
	return File{Path: "index.html", Content: `<html><body><div id="root" data-composition-id="main" data-start="0" data-duration="12" data-width="1080" data-height="1920">` + extra + `</div></body></html>`}
}

func TestBuildAcceptsAMinimalProject(t *testing.T) {
	p, err := Build([]File{idx("")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.DurationMs != 12000 {
		t.Fatalf("DurationMs = %d, want 12000", p.DurationMs)
	}
}

func TestBuildRequiresAnIndex(t *testing.T) {
	_, err := Build([]File{{Path: "compositions/hook.html", Content: "<template></template>"}})
	if err == nil || !strings.Contains(err.Error(), "index.html") {
		t.Fatalf("expected a complaint about index.html, got %v", err)
	}
}

// The root's duration is the film's length and the wallet's input. A scene's
// duration is not, and taking one would both mis-charge and cut the film.
func TestBuildReadsTheROOTDurationNotASceneDuration(t *testing.T) {
	root := File{Path: "index.html", Content: `
<html><body>
  <div id="root" data-composition-id="main" data-start="0" data-duration="20" data-width="1080" data-height="1920">
    <section id="hook" class="clip" data-scene="hook" data-start="0" data-duration="3"></section>
  </div>
</body></html>`}
	p, err := Build([]File{root})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.DurationMs != 20000 {
		t.Fatalf("DurationMs = %d, want 20000 (the root), not 3000 (the first scene)", p.DurationMs)
	}
}

func TestBuildRejectsAMissingDuration(t *testing.T) {
	_, err := Build([]File{{Path: "index.html", Content: `<div id="root" data-composition-id="main"></div>`}})
	if err == nil || !strings.Contains(err.Error(), "data-duration") {
		t.Fatalf("expected a complaint about data-duration, got %v", err)
	}
}

// A mounted scene that is not in the project does not fail in the CLI — it
// waits 45 seconds and renders static frames, which reads as a rendering
// problem rather than a missing file.
// A mounted sub-composition is refused WHETHER OR NOT the file is there. It
// passes `check`, renders without a warning, and produces a film of unstyled
// text — every signal says the design is fine. Failing at authoring, where the
// message reaches the model, costs nothing; failing after a charged render
// costs the user ~2000 tokens and looks like the product is broken.
func TestBuildRefusesAMountedSceneEvenWhenTheFileExists(t *testing.T) {
	_, err := Build([]File{
		idx(`<div data-composition-src="compositions/hook.html"></div>`),
		{Path: "compositions/hook.html", Content: "<template></template>"},
	})
	if err == nil {
		t.Fatal("a mounted sub-composition was accepted; it cannot render here")
	}
	// The message is read by a model whose next turn is the fix, so it has to
	// name both the offending file and the shape to use instead.
	for _, want := range []string{"compositions/hook.html", "data-scene", "index.html"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the refusal does not mention %q: %v", want, err)
		}
	}
}

func TestBuildRefusesAMountedSceneThatIsNotThere(t *testing.T) {
	_, err := Build([]File{idx(`<div data-composition-src="compositions/missing.html"></div>`)})
	if err == nil || !strings.Contains(err.Error(), "compositions/missing.html") {
		t.Fatalf("expected a complaint naming the scene, got %v", err)
	}
}

func TestBuildRejectsPathsThatEscapeTheProject(t *testing.T) {
	for _, bad := range []string{"../secrets.txt", "/etc/passwd", "compositions\\hook.html", "", "   "} {
		_, err := Build([]File{idx(""), {Path: bad, Content: "x"}})
		if err == nil {
			t.Fatalf("path %q was accepted", bad)
		}
	}
}

func TestBuildRejectsDuplicatePaths(t *testing.T) {
	_, err := Build([]File{idx(""), {Path: "a.html", Content: "1"}, {Path: "./a.html", Content: "2"}})
	if err == nil || !strings.Contains(err.Error(), "twice") {
		t.Fatalf("expected a duplicate-path complaint, got %v", err)
	}
}

// A model that loops must not be able to fill the bucket; the failure would
// look like a slow render rather than what it is.
func TestBuildBoundsSizeAndCount(t *testing.T) {
	big := []File{idx(""), {Path: "big.html", Content: strings.Repeat("x", MaxBytes+1)}}
	if _, err := Build(big); err == nil || !strings.Contains(err.Error(), "bytes") {
		t.Fatalf("expected a size limit, got %v", err)
	}

	many := []File{idx("")}
	for i := 0; i < MaxFiles; i++ {
		many = append(many, File{Path: "f" + string(rune('a'+i%26)) + string(rune('a'+i/26)) + ".html", Content: "x"})
	}
	if _, err := Build(many); err == nil || !strings.Contains(err.Error(), "more than") {
		t.Fatalf("expected a file-count limit, got %v", err)
	}
}

func TestPrefixIsScopedToBrandAndDesign(t *testing.T) {
	got := Prefix("b1", "d1")
	if got != "design-projects/b1/d1" {
		t.Fatalf("Prefix = %q", got)
	}
	// The render task role is scoped to design-projects/* — a prefix outside
	// it would be unreadable by the worker, with a 403 at render time.
	if !strings.HasPrefix(got, "design-projects/") {
		t.Fatalf("prefix must stay under design-projects/, got %q", got)
	}
}

// The real index.html the model wrote on dev, which is what the timeline has
// to be drawn from. Shortened only by dropping the variables block.
const realIndex = `<html>
  <body style="margin:0; background: #0F0B08;">
    <div id="root" data-composition-id="main" data-start="0" data-duration="10" data-width="1080" data-height="1920">
      <div id="hook" data-composition-id="hook" data-composition-src="compositions/hook.html" data-start="0" data-duration="3" data-track-index="1" data-track-kind="graphics" data-width="1080" data-height="1920"></div>
      <div id="benefits" data-composition-id="benefits" data-composition-src="compositions/benefits.html" data-start="3" data-duration="4" data-track-index="1" data-track-kind="graphics" data-width="1080" data-height="1920"></div>
      <div id="outro" data-composition-id="outro" data-composition-src="compositions/outro.html" data-start="7" data-duration="3" data-track-index="1" data-track-kind="graphics" data-width="1080" data-height="1920"></div>
    </div>
  </body>
</html>`

func TestParseScenesReadsTheRealProject(t *testing.T) {
	scenes := parseScenes(realIndex)
	if len(scenes) != 3 {
		t.Fatalf("expected the 3 mounted scenes, got %d: %+v", len(scenes), scenes)
	}
	want := []Scene{
		{ID: "hook", Src: "compositions/hook.html", StartMs: 0, DurationMs: 3000, TrackIndex: 1, TrackKind: "graphics"},
		{ID: "benefits", Src: "compositions/benefits.html", StartMs: 3000, DurationMs: 4000, TrackIndex: 1, TrackKind: "graphics"},
		{ID: "outro", Src: "compositions/outro.html", StartMs: 7000, DurationMs: 3000, TrackIndex: 1, TrackKind: "graphics"},
	}
	for i, w := range want {
		if scenes[i] != w {
			t.Fatalf("scene %d = %+v, want %+v", i, scenes[i], w)
		}
	}
}

// The root carries data-duration too, and it is the FILM's length. Counting it
// as a scene would put a 10-second clip on the timeline next to the three
// scenes that make it up.
func TestParseScenesIgnoresTheRootComposition(t *testing.T) {
	for _, s := range parseScenes(realIndex) {
		if s.ID == "main" || s.DurationMs == 10000 {
			t.Fatalf("the root composition was parsed as a scene: %+v", s)
		}
	}
}

// Attributes must come off the SAME element. Scanning the document for
// data-start would pair one scene's start with another's duration, and the
// error would look like a timeline bug rather than a parsing one.
func TestParseScenesDoesNotMixAttributesBetweenElements(t *testing.T) {
	index := `<div data-composition-id="a" data-composition-src="a.html" data-start="0" data-duration="2"></div>` +
		`<div data-composition-id="b" data-composition-src="b.html" data-start="2" data-duration="5"></div>`
	scenes := parseScenes(index)
	if len(scenes) != 2 {
		t.Fatalf("expected 2 scenes, got %d", len(scenes))
	}
	if scenes[0].DurationMs != 2000 || scenes[1].DurationMs != 5000 {
		t.Fatalf("durations crossed over: %+v", scenes)
	}
}

// Document order is not guaranteed to be timeline order, and a timeline that
// runs backwards is worse than none.
func TestParseScenesSortsByStart(t *testing.T) {
	index := `<div data-composition-id="late" data-composition-src="l.html" data-start="7" data-duration="3"></div>` +
		`<div data-composition-id="early" data-composition-src="e.html" data-start="0" data-duration="7"></div>`
	scenes := parseScenes(index)
	if scenes[0].ID != "early" || scenes[1].ID != "late" {
		t.Fatalf("not in timeline order: %+v", scenes)
	}
}

// Everything parsed here feeds a DRAWING, not the render and not the price. A
// project with odd attributes must still render; the user just gets a less
// precise picture.
func TestParseScenesIsForgivingAboutMissingAttributes(t *testing.T) {
	index := `<div data-composition-src="only-src.html"></div>`
	scenes := parseScenes(index)
	if len(scenes) != 1 {
		t.Fatalf("a scene with only a src should still appear, got %+v", scenes)
	}
	if scenes[0].ID != "only-src.html" {
		t.Fatalf("expected the src as the fallback handle, got %q", scenes[0].ID)
	}
	if scenes[0].StartMs != 0 || scenes[0].DurationMs != 0 {
		t.Fatalf("missing times should read as 0, got %+v", scenes[0])
	}
}

func TestParseScenesHandlesAProjectWithNoScenes(t *testing.T) {
	if got := parseScenes(`<div data-composition-id="main" data-duration="5"></div>`); len(got) != 0 {
		t.Fatalf("expected no scenes, got %+v", got)
	}
}

// Hyperframes writes these times in SECONDS. Reading them as milliseconds
// would make every clip a thousand times too long.
func TestParseScenesConvertsSecondsToMilliseconds(t *testing.T) {
	scenes := parseScenes(`<div data-composition-src="a.html" data-start="1.5" data-duration="0.25"></div>`)
	if scenes[0].StartMs != 1500 || scenes[0].DurationMs != 250 {
		t.Fatalf("fractional seconds mis-converted: %+v", scenes[0])
	}
}

// The single-document shape the contract now asks for: one file, scenes as
// timed sections.
const singleDocIndex = `<html>
  <head><style>.scene{position:absolute;inset:0}</style></head>
  <body style="margin:0">
    <div id="root" data-composition-id="main" data-start="0" data-duration="10" data-width="1080" data-height="1920">
      <section id="hook" class="scene clip" data-scene="hook" data-start="0" data-duration="3"></section>
      <section id="benefits" class="scene clip" data-scene="benefits" data-start="3" data-duration="4"></section>
      <section id="outro" class="scene clip" data-scene="outro" data-start="7" data-duration="3"></section>
    </div>
    <script>window.__timelines["main"] = gsap.timeline({ paused: true });</script>
  </body>
</html>`

func TestBuildCarriesTheScenes(t *testing.T) {
	p, err := Build([]File{{Path: "index.html", Content: singleDocIndex}})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if p.DurationMs != 10000 {
		t.Fatalf("root duration = %d, want 10000", p.DurationMs)
	}
	if len(p.Scenes) != 3 {
		t.Fatalf("Build dropped the scenes: %+v", p.Scenes)
	}
}

func TestParseScenesReadsTheSingleDocumentShape(t *testing.T) {
	scenes := parseScenes(singleDocIndex)
	if len(scenes) != 3 {
		t.Fatalf("expected 3 scenes, got %d: %+v", len(scenes), scenes)
	}
	want := []Scene{
		{ID: "hook", StartMs: 0, DurationMs: 3000},
		{ID: "benefits", StartMs: 3000, DurationMs: 4000},
		{ID: "outro", StartMs: 7000, DurationMs: 3000},
	}
	for i, w := range want {
		if scenes[i].ID != w.ID || scenes[i].StartMs != w.StartMs || scenes[i].DurationMs != w.DurationMs {
			t.Fatalf("scene %d = %+v, want %+v", i, scenes[i], w)
		}
	}
}

// The ROOT carries data-duration too, and it is the FILM's length. It has no
// data-scene, which is what keeps it off the timeline — a 10-second clip beside
// the three scenes that make it up would be nonsense.
func TestParseScenesIgnoresTheRootInTheSingleDocumentShape(t *testing.T) {
	for _, s := range parseScenes(singleDocIndex) {
		if s.ID == "main" || s.DurationMs == 10000 {
			t.Fatalf("the root was parsed as a scene: %+v", s)
		}
	}
}

// A design authored before the contract moved to one document still has
// mounted sub-compositions. It can no longer render, but its timeline is
// still worth drawing rather than silently emptying.
func TestParseScenesStillReadsLegacyMountedProjects(t *testing.T) {
	scenes := parseScenes(realIndex)
	if len(scenes) != 3 {
		t.Fatalf("legacy project lost its scenes: %+v", scenes)
	}
	if scenes[0].ID != "hook" || scenes[0].Src != "compositions/hook.html" {
		t.Fatalf("legacy scene mis-parsed: %+v", scenes[0])
	}
}

// The current shape wins when both are present, so a half-migrated project
// does not draw every scene twice.
func TestParseScenesPrefersTheCurrentShapeOverTheLegacyOne(t *testing.T) {
	mixed := `<div data-scene="a" data-start="0" data-duration="2"></div>` +
		`<div data-composition-id="b" data-composition-src="b.html" data-start="2" data-duration="2"></div>`
	scenes := parseScenes(mixed)
	if len(scenes) != 1 || scenes[0].ID != "a" {
		t.Fatalf("expected only the data-scene element, got %+v", scenes)
	}
}

// A section carrying times but no data-scene is a timed element, not a scene.
// Counting it would put every animated card on the timeline as its own clip.
func TestParseScenesNeedsTheSceneMarker(t *testing.T) {
	if got := parseScenes(`<div class="clip" data-start="0" data-duration="2"></div>`); len(got) != 0 {
		t.Fatalf("a clip without data-scene became a scene: %+v", got)
	}
}

// A scene timed with data-end has a length, and it is the same length
// clips.go reports for it.
//
// Two readings of one document that disagree is how a scene came to be
// three seconds long on the timeline and zero in the scene panel.
func TestSceneDurationFromDataEnd(t *testing.T) {
	const doc = `<div id="root" data-composition-id="reel" data-duration="5">
       <section id="s1" class="clip" data-scene="hook" data-start="0" data-end="3"></section>
       <section id="s2" class="clip" data-scene="end" data-start="3" data-duration="2"></section>
     </div>`

	scenes := parseScenes(doc)
	if len(scenes) != 2 {
		t.Fatalf("want two scenes, got %d", len(scenes))
	}
	if scenes[0].DurationMs != 3000 {
		t.Fatalf("data-end should give the scene a length: got %d, want 3000", scenes[0].DurationMs)
	}
	if scenes[1].DurationMs != 2000 {
		t.Fatalf("data-duration still wins where it is given: got %d", scenes[1].DurationMs)
	}

	// And the timeline agrees with the panel, which is the point.
	for _, c := range Clips(doc) {
		if c.Kind != ClipScene {
			continue
		}
		for _, s := range scenes {
			if s.ID == c.ElementID && s.DurationMs != c.DurationMs {
				t.Fatalf("scene %q: panel says %d, timeline says %d", s.ID, s.DurationMs, c.DurationMs)
			}
		}
	}
}
