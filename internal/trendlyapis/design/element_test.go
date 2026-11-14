package design

import (
	"strings"
	"testing"
)

const filmHTML = `<!doctype html>
<html><head><style>.clip{opacity:0}</style></head>
<body>
  <div id="root" data-composition-id="main" data-duration="9">
    <section id="s1" data-scene="hook">
      <h1 id="title" class="clip" data-start="0" data-duration="3" style="color:#fff;font-size:88px;transform:translateY(10px)">Hit the 2 PM wall?</h1>
      <p id="sub" class="clip" data-start="0.5" data-duration="2.5">Cold brew, no sugar</p>
    </section>
  </div>
  <script>gsap.timeline();</script>
</body></html>`

func str(s string) *string { return &s }
func num(n int) *int       { return &n }

func TestEditTheWords(t *testing.T) {
	out, err := applyElementEdit(filmHTML, editElementRequest{ElementID: "title", Text: str("Tired of sugar?")})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, ">Tired of sugar?</h1>") {
		t.Fatalf("the words did not change:\n%s", out)
	}
	// And nothing else did.
	if !strings.Contains(out, `style="color:#fff;font-size:88px;transform:translateY(10px)"`) {
		t.Fatal("the style was touched by a text edit")
	}
	if !strings.Contains(out, "Cold brew, no sugar") {
		t.Fatal("a different element changed")
	}
}

// The reason style is MERGED and not replaced: the panel has a colour
// control and no transform control, and an edit to one must not throw away
// the other.
func TestStyleMergeKeepsWhatThePanelCannotSee(t *testing.T) {
	out, err := applyElementEdit(filmHTML, editElementRequest{
		ElementID: "title",
		Style:     map[string]string{"color": "#054463"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "transform:translateY(10px)") {
		t.Fatalf("the transform was dropped:\n%s", out)
	}
	if !strings.Contains(out, "color:#054463") || strings.Contains(out, "color:#fff") {
		t.Fatalf("the colour did not change:\n%s", out)
	}
	if !strings.Contains(out, "font-size:88px") {
		t.Fatal("the size was dropped")
	}
}

func TestAnEmptyValueRemovesAProperty(t *testing.T) {
	out, err := applyElementEdit(filmHTML, editElementRequest{
		ElementID: "title",
		Style:     map[string]string{"font-size": ""},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "font-size") {
		t.Fatalf("the property was not removed:\n%s", out)
	}
	if !strings.Contains(out, "color:#fff") {
		t.Fatal("removing one property removed another")
	}
}

// Two identical requests must produce identical documents, or a revision
// differs from the last only in declaration order and nobody can read the
// diff.
func TestTheSameEditTwiceProducesTheSameDocument(t *testing.T) {
	req := editElementRequest{
		ElementID: "title",
		Style: map[string]string{
			"color": "#054463", "background": "#fff", "font-weight": "700",
			"letter-spacing": "0.02em", "text-align": "center",
		},
	}
	first, err := applyElementEdit(filmHTML, req)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		again, err := applyElementEdit(filmHTML, req)
		if err != nil {
			t.Fatal(err)
		}
		if again != first {
			t.Fatalf("map iteration leaked into the output:\n%s\n---\n%s", first, again)
		}
	}
}

func TestTimingIsWrittenInSeconds(t *testing.T) {
	out, err := applyElementEdit(filmHTML, editElementRequest{
		ElementID: "title", StartMs: num(1500), DurationMs: num(4000),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `data-start="1.5"`) {
		t.Fatalf("start is wrong:\n%s", out)
	}
	// Whole seconds do not print as "4.00".
	if !strings.Contains(out, `data-duration="4"`) {
		t.Fatalf("duration is wrong:\n%s", out)
	}
}

func TestTimingRefusesWhatCannotBePlayed(t *testing.T) {
	if _, err := applyElementEdit(filmHTML, editElementRequest{ElementID: "title", StartMs: num(-1)}); err == nil {
		t.Fatal("accepted a negative start")
	}
	if _, err := applyElementEdit(filmHTML, editElementRequest{ElementID: "title", DurationMs: num(0)}); err == nil {
		t.Fatal("accepted a clip of no length")
	}
}

func TestAnElementThatIsNoLongerThere(t *testing.T) {
	_, err := applyElementEdit(filmHTML, editElementRequest{ElementID: "gone", Text: str("x")})
	if err == nil || !strings.Contains(err.Error(), "no longer in this film") {
		t.Fatalf("error was %v", err)
	}
}

// A declaration assembled from a request body is exactly where a stray
// character turns one property into two, or ends the attribute.
func TestStyleValuesThatWouldEscapeTheDeclaration(t *testing.T) {
	bad := map[string]string{
		"red;position:fixed;top:0":            "a second property",
		`#fff" onload="alert(1)`:              "ending the attribute",
		"url(https://example.com/x.png)":      "fetching something",
		"expression(alert(1))":                "legacy script",
		"javascript:alert(1)":                 "a script url",
		"#fff/*":                              "an open comment",
		"<script>":                            "markup",
	}
	for value, why := range bad {
		_, err := applyElementEdit(filmHTML, editElementRequest{
			ElementID: "title", Style: map[string]string{"color": value},
		})
		if err == nil {
			t.Fatalf("accepted %q (%s)", value, why)
		}
	}
}

func TestStyleNamesThatAreNotProperties(t *testing.T) {
	for _, name := range []string{"color;x", "color:red", "", "COLOR!", "a b"} {
		_, err := applyElementEdit(filmHTML, editElementRequest{
			ElementID: "title", Style: map[string]string{name: "#fff"},
		})
		if err == nil {
			t.Fatalf("accepted %q as a property name", name)
		}
	}
	// Uppercase is a spelling, not an attack: it is lowercased and kept.
	out, err := applyElementEdit(filmHTML, editElementRequest{
		ElementID: "title", Style: map[string]string{"COLOR": "#054463"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "color:#054463") {
		t.Fatalf("an uppercase property name was not folded:\n%s", out)
	}
}

func TestTextAndStyleAndTimingInOneEdit(t *testing.T) {
	out, err := applyElementEdit(filmHTML, editElementRequest{
		ElementID:  "title",
		Text:       str("Tired of sugar?"),
		Style:      map[string]string{"color": "#054463"},
		StartMs:    num(500),
		DurationMs: num(2500),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{">Tired of sugar?</h1>", "color:#054463", `data-start="0.5"`, `data-duration="2.5"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
}

func TestEditNoteSaysWhatChanged(t *testing.T) {
	note := editNote(editElementRequest{
		ElementID: "title", Text: str("x"),
		Style:   map[string]string{"font-size": "90px", "color": "#000"},
		StartMs: num(0),
	})
	if note != "title · text, color, font-size, timing" {
		t.Fatalf("note is %q", note)
	}
}
