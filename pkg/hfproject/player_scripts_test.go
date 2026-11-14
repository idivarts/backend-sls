package hfproject

import (
	"strings"
	"testing"
)

const runtimeSrc = "https://cdn.example.com/design-player/0.9.39/runtime.js"
const gsapSrc = "https://cdn.example.com/design-player/0.9.39/gsap-3.15.0.min.js"

const plainFilm = `<!doctype html>
<html><head><style>.clip{opacity:0}</style></head>
<body><div id="root" data-composition-id="main" data-duration="3"></div>
<script>gsap.timeline();</script>
</body></html>`

func countScript(html, src string) int {
	return strings.Count(html, `src="`+src+`"`)
}

func TestScriptsGoInOnce(t *testing.T) {
	out := withScripts(plainFilm, runtimeSrc, gsapSrc)
	if got := countScript(out, runtimeSrc); got != 1 {
		t.Fatalf("runtime appears %d times", got)
	}
	if got := countScript(out, gsapSrc); got != 1 {
		t.Fatalf("gsap appears %d times", got)
	}
}

// The regression this exists for. index.html is the ONE document, so what
// this writes is what the next edit reads: four edits on dev left five
// copies of the runtime in a film's head, each re-executing the player
// over the one before it.
func TestInjectingAgainDoesNotAddASecondCopy(t *testing.T) {
	out := plainFilm
	for i := 0; i < 6; i++ {
		out = withScripts(out, runtimeSrc, gsapSrc)
	}
	if got := countScript(out, runtimeSrc); got != 1 {
		t.Fatalf("after six passes the runtime appears %d times", got)
	}
	if got := countScript(out, gsapSrc); got != 1 {
		t.Fatalf("after six passes gsap appears %d times", got)
	}
}

// And a film that already collected a few comes out with one.
func TestADocumentThatAlreadyCollectedCopiesIsRepaired(t *testing.T) {
	hurt := strings.Replace(plainFilm, "<head>",
		"<head>\n    <script src=\""+runtimeSrc+"\"></script>\n    <script src=\""+runtimeSrc+"\"></script>\n    <script src=\""+gsapSrc+"\"></script>", 1)
	if countScript(hurt, runtimeSrc) != 2 {
		t.Fatal("the fixture is wrong")
	}
	out := withScripts(hurt, runtimeSrc, gsapSrc)
	if got := countScript(out, runtimeSrc); got != 1 {
		t.Fatalf("the repaired document has %d runtimes", got)
	}
}

// The film's OWN scripts are never touched — that is its timeline.
func TestTheFilmsOwnScriptSurvives(t *testing.T) {
	out := withScripts(plainFilm, runtimeSrc, gsapSrc)
	if !strings.Contains(out, "<script>gsap.timeline();</script>") {
		t.Fatalf("the film's timeline was removed:\n%s", out)
	}
	// Nor is a different src removed.
	other := strings.Replace(plainFilm, "</head>", `<script src="https://cdn.example.com/other.js"></script></head>`, 1)
	out = withScripts(other, runtimeSrc, gsapSrc)
	if !strings.Contains(out, "other.js") {
		t.Fatal("an unrelated script was removed")
	}
}
