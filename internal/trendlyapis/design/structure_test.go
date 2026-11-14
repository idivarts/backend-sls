package design

import (
	"regexp"
	"strings"
	"testing"
)

const threeScenes = `<!doctype html>
<html><body>
  <div id="root" data-composition-id="main" data-duration="9">
    <section id="hook" class="clip" data-scene="hook" data-start="0" data-duration="3">
      <h1 id="hook-title" class="clip" data-start="0.2" data-duration="2.6">One</h1>
    </section>
    <section id="middle" class="clip" data-scene="middle" data-start="3" data-duration="3">
      <h1 id="middle-title" class="clip" data-start="3.2" data-duration="2.6">Two</h1>
    </section>
    <section id="end" class="clip" data-scene="end" data-start="6" data-duration="3">
      <h1 id="end-title" class="clip" data-start="6.2" data-duration="2.6">Three</h1>
    </section>
  </div>
  <script>gsap.timeline();</script>
</body></html>`

func num2(n int) *int { return &n }

// attr reads one attribute off the element with this id.
func attr(t *testing.T, html, id, name string) string {
	t.Helper()
	re := regexp.MustCompile(`<[a-z0-9]+[^>]*\bid="` + regexp.QuoteMeta(id) + `"[^>]*>`)
	tag := re.FindString(html)
	if tag == "" {
		// The id may come before other attributes; try the other order.
		re = regexp.MustCompile(`<[a-z0-9]+[^>]*\b` + regexp.QuoteMeta(`id="`+id+`"`) + `[^>]*>`)
		tag = re.FindString(html)
	}
	if tag == "" {
		t.Fatalf("no element with id %q", id)
	}
	m := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `="([^"]*)"`).FindStringSubmatch(tag)
	if m == nil {
		return ""
	}
	return m[1]
}

func TestRetimeRipplesEverythingAfterIt(t *testing.T) {
	out, total, err := applyStructureEdit(threeScenes, structureRequest{
		Op: sceneOpRetime, SceneID: "hook", DurationMs: num2(5000),
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 11000 {
		t.Fatalf("film is %dms, want 11000", total)
	}
	if got := attr(t, out, "hook", "data-duration"); got != "5" {
		t.Fatalf("hook lasts %q", got)
	}
	if got := attr(t, out, "middle", "data-start"); got != "5" {
		t.Fatalf("middle starts at %q, want 5 — the ripple did not happen", got)
	}
	if got := attr(t, out, "end", "data-start"); got != "8" {
		t.Fatalf("end starts at %q, want 8", got)
	}
	if got := attr(t, out, "root", "data-duration"); got != "11" {
		t.Fatalf("the root says %q, want 11 — this is what the wallet is priced from", got)
	}
}

// Timing in these films is absolute, so a scene's contents have to move
// with it or they play during the scene before.
func TestAChildMovesWithItsScene(t *testing.T) {
	out, _, err := applyStructureEdit(threeScenes, structureRequest{
		Op: sceneOpRetime, SceneID: "hook", DurationMs: num2(5000),
	})
	if err != nil {
		t.Fatal(err)
	}
	// middle moved 3s → 5s, so its headline moved 3.2s → 5.2s.
	if got := attr(t, out, "middle-title", "data-start"); got != "5.2" {
		t.Fatalf("the headline is at %q, want 5.2 — it was left behind", got)
	}
	// The hook did not move, so its headline did not either.
	if got := attr(t, out, "hook-title", "data-start"); got != "0.2" {
		t.Fatalf("a headline moved when its scene did not: %q", got)
	}
}

func TestMoveRewritesTheDocumentAndTheClock(t *testing.T) {
	out, total, err := applyStructureEdit(threeScenes, structureRequest{
		Op: sceneOpMove, SceneID: "end", ToIndex: num2(0),
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 9000 {
		t.Fatalf("moving changed the length to %d", total)
	}
	// The document order follows, or everything that reads the project
	// disagrees with the film.
	first, second, third := strings.Index(out, `id="end"`), strings.Index(out, `id="hook"`), strings.Index(out, `id="middle"`)
	if !(first < second && second < third) {
		t.Fatalf("the markup was not reordered: end=%d hook=%d middle=%d", first, second, third)
	}
	if got := attr(t, out, "end", "data-start"); got != "0" {
		t.Fatalf("the moved scene starts at %q", got)
	}
	if got := attr(t, out, "hook", "data-start"); got != "3" {
		t.Fatalf("hook starts at %q", got)
	}
	// And its headline came with it.
	if got := attr(t, out, "end-title", "data-start"); got != "0.2" {
		t.Fatalf("the moved scene's headline is at %q, want 0.2", got)
	}
}

func TestDeleteClosesTheGap(t *testing.T) {
	out, total, err := applyStructureEdit(threeScenes, structureRequest{Op: sceneOpDelete, SceneID: "middle"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, `id="middle"`) {
		t.Fatal("the scene is still there")
	}
	if total != 6000 {
		t.Fatalf("the film is %dms, want 6000", total)
	}
	if got := attr(t, out, "end", "data-start"); got != "3" {
		t.Fatalf("end starts at %q, want 3 — the gap was left open", got)
	}
	if got := attr(t, out, "end-title", "data-start"); got != "3.2" {
		t.Fatalf("the headline is at %q, want 3.2", got)
	}
}

func TestTheLastSceneCannotBeDeleted(t *testing.T) {
	one := `<div id="root" data-composition-id="main" data-duration="3">
	  <section id="only" data-scene="only" data-start="0" data-duration="3"><h1 id="t">A</h1></section></div>`
	if _, _, err := applyStructureEdit(one, structureRequest{Op: sceneOpDelete, SceneID: "only"}); err == nil {
		t.Fatal("deleted the only scene")
	}
}

// Two elements sharing an id make the bridge, the inspector and every
// later edit ambiguous.
func TestDuplicateGivesEveryIdInsideItANewOne(t *testing.T) {
	out, total, err := applyStructureEdit(threeScenes, structureRequest{Op: sceneOpDuplicate, SceneID: "hook"})
	if err != nil {
		t.Fatal(err)
	}
	if total != 12000 {
		t.Fatalf("the film is %dms, want 12000", total)
	}
	if strings.Count(out, `id="hook"`) != 1 {
		t.Fatalf("the original id appears %d times", strings.Count(out, `id="hook"`))
	}
	if !strings.Contains(out, `id="hook-2"`) || !strings.Contains(out, `id="hook-title-2"`) {
		t.Fatalf("the copy did not get fresh ids:\n%s", out)
	}
	// The copy plays after the original, and the rest ripples.
	if got := attr(t, out, "hook-2", "data-start"); got != "3" {
		t.Fatalf("the copy starts at %q, want 3", got)
	}
	if got := attr(t, out, "middle", "data-start"); got != "6" {
		t.Fatalf("middle starts at %q, want 6", got)
	}
}

func TestAddPutsANewSceneAtTheEnd(t *testing.T) {
	out, total, err := applyStructureEdit(threeScenes, structureRequest{Op: sceneOpAdd})
	if err != nil {
		t.Fatal(err)
	}
	if total != 12000 {
		t.Fatalf("the film is %dms, want 12000", total)
	}
	if !strings.Contains(out, "New scene") {
		t.Fatal("the new scene has nothing in it, which renders as a bug")
	}
	if got := attr(t, out, "scene-4", "data-start"); got != "9" {
		t.Fatalf("the new scene starts at %q, want 9", got)
	}
}

func TestRefusals(t *testing.T) {
	cases := []struct {
		req  structureRequest
		what string
	}{
		{structureRequest{Op: "explode", SceneID: "hook"}, "an operation that does not exist"},
		{structureRequest{Op: sceneOpRetime, SceneID: "gone", DurationMs: num2(3000)}, "a scene that is not there"},
		{structureRequest{Op: sceneOpRetime, SceneID: "hook", DurationMs: num2(100)}, "a scene too short to read"},
		{structureRequest{Op: sceneOpRetime, SceneID: "hook"}, "no duration at all"},
		{structureRequest{Op: sceneOpMove, SceneID: "hook", ToIndex: num2(9)}, "a position that does not exist"},
		{structureRequest{Op: sceneOpMove, SceneID: "hook", ToIndex: num2(0)}, "a move to where it already is"},
	}
	for _, c := range cases {
		if _, _, err := applyStructureEdit(threeScenes, c.req); err == nil {
			t.Fatalf("accepted %s", c.what)
		}
	}
}

// Nothing else in the document may move: the stylesheet, the script and
// the untouched scenes come through byte for byte.
func TestAStructuralEditLeavesEverythingElseAlone(t *testing.T) {
	out, _, err := applyStructureEdit(threeScenes, structureRequest{
		Op: sceneOpRetime, SceneID: "end", DurationMs: num2(4000),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, untouched := range []string{
		`<script>gsap.timeline();</script>`,
		`<h1 id="hook-title" class="clip" data-start="0.2" data-duration="2.6">One</h1>`,
	} {
		if !strings.Contains(out, untouched) {
			t.Fatalf("something else changed; missing:\n%s", untouched)
		}
	}
}

// The collision the suffix search exists to avoid: a film that already
// holds the id a copy would produce for one of the scene's CHILDREN.
func TestDuplicateAvoidsAnIdAChildWouldCollideWith(t *testing.T) {
	// hook-2 is free, but hook-title-2 is taken — by a scene a previous
	// duplicate left behind, or by hand.
	film := strings.Replace(threeScenes,
		`<h1 id="end-title" class="clip" data-start="6.2" data-duration="2.6">Three</h1>`,
		`<h1 id="hook-title-2" class="clip" data-start="6.2" data-duration="2.6">Three</h1>`, 1)

	out, _, err := applyStructureEdit(film, structureRequest{Op: sceneOpDuplicate, SceneID: "hook"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, `id="hook-title-2"`) != 1 {
		t.Fatalf("the copy collided with an id already in the film:\n%s", out)
	}
	// It took the next suffix for the whole copy, so the scene and its
	// child agree.
	if !strings.Contains(out, `id="hook-3"`) || !strings.Contains(out, `id="hook-title-3"`) {
		t.Fatalf("the copy did not move to a free suffix:\n%s", out)
	}
}

// What the app calls a scene is its data-scene value; what the document
// calls it is its id. They are not the same string, and the panel only
// knows the first — so every button in it answered "that scene is no
// longer in this film" about a scene plainly on screen.
func TestASceneIsFoundByEitherOfItsNames(t *testing.T) {
	film := `<div id="root" data-composition-id="main" data-duration="6">
	  <section id="scene-coldbrew" data-scene="coldbrew" data-start="0" data-duration="3"><h1 id="a">A</h1></section>
	  <section id="scene-nosugar" data-scene="nosugar" data-start="3" data-duration="3"><h1 id="b">B</h1></section>
	</div>`

	for _, key := range []string{"coldbrew", "scene-coldbrew"} {
		out, total, err := applyStructureEdit(film, structureRequest{
			Op: sceneOpRetime, SceneID: key, DurationMs: num2(5000),
		})
		if err != nil {
			t.Fatalf("%q: %v", key, err)
		}
		if total != 8000 {
			t.Fatalf("%q: film is %dms, want 8000", key, total)
		}
		if got := attr(t, out, "scene-nosugar", "data-start"); got != "5" {
			t.Fatalf("%q: the ripple did not happen (%s)", key, got)
		}
	}

	// And media goes into the scene the app named.
	out, note, err := applyMediaPlacement(film, placeMediaRequest{
		SceneID: "nosugar", Type: "bgm", Intent: "upbeat lo-fi",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(note, "nosugar") {
		t.Fatalf("the note says %q", note)
	}
	if strings.Index(out, `id="bgm-1"`) < strings.Index(out, `id="scene-nosugar"`) {
		t.Fatal("the music landed in the wrong scene")
	}
}

// A copy that kept the original's data-scene would give the panel two
// rows called "coldbrew" and no way to say which one a button meant:
// pressing Delete on the second would delete the first.
func TestADuplicateGetsItsOwnName(t *testing.T) {
	film := `<div id="root" data-composition-id="main" data-duration="6">
	  <section id="scene-coldbrew" data-scene="coldbrew" data-start="0" data-duration="3"><h1 id="a">A</h1></section>
	  <section id="scene-nosugar" data-scene="nosugar" data-start="3" data-duration="3"><h1 id="b">B</h1></section>
	</div>`
	out, _, err := applyStructureEdit(film, structureRequest{Op: sceneOpDuplicate, SceneID: "coldbrew"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, `data-scene="coldbrew"`) != 1 {
		t.Fatalf("two scenes share a name:\n%s", out)
	}
	if !strings.Contains(out, `data-scene="coldbrew-2"`) {
		t.Fatalf("the copy was not renamed:\n%s", out)
	}

	// And each row now addresses its own scene: deleting the copy leaves
	// the original.
	after, total, err := applyStructureEdit(out, structureRequest{Op: sceneOpDelete, SceneID: "coldbrew-2"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(after, `data-scene="coldbrew"`) || strings.Contains(after, `coldbrew-2`) {
		t.Fatalf("the wrong scene was deleted:\n%s", after)
	}
	if total != 6000 {
		t.Fatalf("the film is %dms, want 6000", total)
	}
}

// An id wins over a name, which is the only way to tell apart two scenes
// in a film that was written with the same name twice.
func TestAnIdBeatsANameWhenBothMatch(t *testing.T) {
	film := `<div id="root" data-composition-id="main" data-duration="6">
	  <section id="first" data-scene="beat" data-start="0" data-duration="3"><h1 id="a">A</h1></section>
	  <section id="second" data-scene="beat" data-start="3" data-duration="3"><h1 id="b">B</h1></section>
	</div>`
	out, _, err := applyStructureEdit(film, structureRequest{Op: sceneOpDelete, SceneID: "second"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `id="first"`) || strings.Contains(out, `id="second"`) {
		t.Fatalf("the wrong scene went:\n%s", out)
	}
}
