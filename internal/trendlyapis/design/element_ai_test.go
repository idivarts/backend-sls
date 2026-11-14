package design

import (
	"strings"
	"testing"

	"github.com/idivarts/backend-sls/pkg/hfedit"
)

// Every one of these is a reply a model will eventually produce, and every
// one of them is the difference between an edit and a silent re-roll.
func TestParseElementReplyRefusesWhatIsNotTheElement(t *testing.T) {
	cases := map[string]string{
		"":                       "an empty answer",
		"   ":                    "whitespace",
		"Sure! Here you go:":     "a sentence with no markup",
		`<h1 id="other">x</h1>`:  "a different element's id",
		`<h1>x</h1>`:             "no id at all",
		`<!doctype html><html><body><h1 id="title">x</h1></body></html>`: "the whole film",
		`<h1 id="title">a</h1><h1 id="title">b</h1>`:                     "the id twice",
		`<div><h1 id="title">x</h1></div>`:                               "a wrapper nobody asked for",
		`<h1 id="title">x</h1><p>and this</p>`:                           "a second element",
		"Here it is:\n<h1 id=\"title\">x</h1>":                           "a preamble",
	}
	for reply, why := range cases {
		if _, err := parseElementReply(reply, "title", "h1"); err == nil {
			t.Fatalf("accepted %q (%s)", reply, why)
		}
	}
}

func TestParseElementReplyTakesACleanAnswer(t *testing.T) {
	reply := `<h1 id="title" class="clip" data-start="0" data-duration="3" style="font-size:120px">Tired of sugar?</h1>`
	out, err := parseElementReply(reply, "title", "h1")
	if err != nil {
		t.Fatal(err)
	}
	if out != reply {
		t.Fatalf("answer was altered:\n%s", out)
	}
}

// Every model fences code sometimes. Refusing that would burn the org's
// tokens on a formatting habit.
func TestParseElementReplyUnwrapsAFence(t *testing.T) {
	for _, reply := range []string{
		"```html\n<h1 id=\"title\">Tired of sugar?</h1>\n```",
		"```\n<h1 id=\"title\">Tired of sugar?</h1>\n```",
	} {
		out, err := parseElementReply(reply, "title", "h1")
		if err != nil {
			t.Fatalf("%q: %v", reply, err)
		}
		if out != `<h1 id="title">Tired of sugar?</h1>` {
			t.Fatalf("unwrapped to %q", out)
		}
	}
}

func TestParseElementReplyAllowsADifferentTag(t *testing.T) {
	// "make this a paragraph" is a legitimate instruction.
	out, err := parseElementReply(`<p id="title">Tired of sugar?</p>`, "title", "h1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "<p ") {
		t.Fatalf("tag change was rejected or mangled: %q", out)
	}
}

// The whole claim of a scoped edit: whatever the model wrote, only the
// element's own bytes move.
func TestAScopedEditCannotTouchAnotherScene(t *testing.T) {
	const film = `<!doctype html>
<html><body>
  <div id="root" data-composition-id="main" data-duration="6">
    <section id="s1" data-scene="hook"><h1 id="title" class="clip">Old words</h1></section>
    <section id="s2" data-scene="end"><h1 id="signoff" class="clip">Sign off</h1></section>
  </div>
  <script>gsap.timeline();</script>
</body></html>`

	el, ok := hfedit.FindByID(film, "title")
	if !ok {
		t.Fatal("no #title")
	}
	replacement, err := parseElementReply(`<h1 id="title" class="clip" style="font-size:140px">New words</h1>`, "title", "h1")
	if err != nil {
		t.Fatal(err)
	}
	out, err := hfedit.Apply(film, []hfedit.Edit{{Start: el.Start, End: el.End, Text: replacement}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "New words") {
		t.Fatal("the edit did not land")
	}
	// The other scene, the script and the root are untouched, byte for byte.
	for _, untouched := range []string{
		`<section id="s2" data-scene="end"><h1 id="signoff" class="clip">Sign off</h1></section>`,
		`<div id="root" data-composition-id="main" data-duration="6">`,
		`<script>gsap.timeline();</script>`,
	} {
		if !strings.Contains(out, untouched) {
			t.Fatalf("a scoped edit changed something else; missing:\n%s", untouched)
		}
	}
}

func TestSceneContextIsTheSceneTheElementIsIn(t *testing.T) {
	const film = `<div id="root">
<section id="s1" data-scene="hook"><h1 id="title">A</h1><p id="sub">B</p></section>
<section id="s2" data-scene="end"><h1 id="signoff">C</h1></section>
</div>`
	el, _ := hfedit.FindByID(film, "title")
	scene := sceneContext(film, el)
	if !strings.Contains(scene, `id="sub"`) {
		t.Fatalf("the scene did not include its other children:\n%s", scene)
	}
	if strings.Contains(scene, `id="signoff"`) {
		t.Fatalf("the context leaked into another scene:\n%s", scene)
	}
}

func TestSceneContextIsEmptyForAnElementOutsideAnyScene(t *testing.T) {
	const film = `<div id="root"><h1 id="loose">A</h1></div>`
	el, _ := hfedit.FindByID(film, "loose")
	if got := sceneContext(film, el); got != "" {
		t.Fatalf("expected no scene, got %q", got)
	}
}

// A scene bigger than the budget is sent as nothing rather than as half a
// scene cut mid-tag, which would be worse context than none.
func TestSceneContextIsDroppedWhenItIsTooBig(t *testing.T) {
	huge := strings.Repeat("<p>filler filler filler</p>", 500)
	film := `<section id="s1" data-scene="hook"><h1 id="title">A</h1>` + huge + `</section>`
	el, _ := hfedit.FindByID(film, "title")
	if got := sceneContext(film, el); got != "" {
		t.Fatalf("a %d-byte scene was sent", len(got))
	}
}

func TestThePromptStatesTheRulesTheParserEnforces(t *testing.T) {
	p := elementEditSystemPrompt(`<section data-scene="hook">…</section>`, `<h1 id="title">A</h1>`, "title", "h1")
	for _, must := range []string{`id="title"`, "<h1>", "data-start", "ONLY"} {
		if !strings.Contains(p, must) {
			t.Fatalf("the prompt never mentions %q", must)
		}
	}
	// The scene is marked as context so the model does not return it.
	if !strings.Contains(p, "do not return it") {
		t.Fatal("the prompt does not say the scene is context only")
	}
}
