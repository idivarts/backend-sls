package hfedit

import (
	"strings"
	"testing"
)

// A film, with the things that break naive editing in it: a GSAP timeline
// full of quotes and angle brackets, CSS with a > in a selector, an
// attribute value that contains the word of another attribute, a void
// element, and a self-closed one.
const film = `<!doctype html>
<html>
<head>
<style>
  .scene > h1 { color: #fff; }
  .clip[data-start] { opacity: 0 }
</style>
</head>
<body>
  <div id="root" data-composition-id="main" data-duration="9">
    <section id="s1" data-scene="hook">
      <h1 id="title" class="clip" data-start="0" data-duration="3" style="color:#fff;font-size:88px">Hit the 2 PM wall?</h1>
      <img id="shot" class="clip" data-media-resolved="assets/img.jpg" src="assets/img.jpg" />
      <br>
      <p id="sub" data-note="style=&quot;color:red&quot; is text, not markup">Cold brew, no sugar</p>
    </section>
  </div>
  <script>
    // Not markup: <h1 id="title">decoy</h1>
    const sel = "<section id='s1'>";
    if (a < b && c > d) { gsap.to("#title", { opacity: 1 }); }
  </script>
</body>
</html>`

func TestFindByIDFindsTheRealElementAndNotTheOneInTheScript(t *testing.T) {
	el, ok := FindByID(film, "title")
	if !ok {
		t.Fatal("did not find #title")
	}
	if el.TagName != "h1" {
		t.Fatalf("tag is %q, want h1", el.TagName)
	}
	if got := film[el.TagEnd : el.End-len("</h1>")]; got != "Hit the 2 PM wall?" {
		t.Fatalf("children are %q, want the headline — the decoy in the <script> was matched", got)
	}
	if el.Attrs["data-duration"] != "3" || el.Attrs["class"] != "clip" {
		t.Fatalf("attrs are %v", el.Attrs)
	}
}

func TestFindByIDOnAVoidAndASelfClosedElement(t *testing.T) {
	img, ok := FindByID(film, "shot")
	if !ok {
		t.Fatal("did not find #shot")
	}
	if img.HasChildren() {
		t.Fatal("a self-closed <img> reported children")
	}
	if img.Attrs["src"] != "assets/img.jpg" {
		t.Fatalf("src is %q", img.Attrs["src"])
	}
}

func TestFindByIDMissing(t *testing.T) {
	if _, ok := FindByID(film, "nope"); ok {
		t.Fatal("found an element that is not there")
	}
	if _, ok := FindByID(film, ""); ok {
		t.Fatal("an empty id matched something")
	}
}

func TestSetTextReplacesOnlyTheChildren(t *testing.T) {
	el, _ := FindByID(film, "title")
	e, err := SetText(el, "Tired of sugar?")
	if err != nil {
		t.Fatal(err)
	}
	out, err := Apply(film, []Edit{e})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `<h1 id="title" class="clip" data-start="0" data-duration="3" style="color:#fff;font-size:88px">Tired of sugar?</h1>`) {
		t.Fatalf("the start tag did not survive verbatim:\n%s", out)
	}
	// And nothing else moved.
	if !strings.Contains(out, `if (a < b && c > d)`) {
		t.Fatal("the script was rewritten")
	}
	if !strings.Contains(out, ".scene > h1 { color: #fff; }") {
		t.Fatal("the stylesheet was rewritten")
	}
}

func TestSetTextEscapesWhatWouldBecomeMarkup(t *testing.T) {
	el, _ := FindByID(film, "title")
	e, _ := SetText(el, `Tom & Jerry <3 "cold brew"`)
	out, _ := Apply(film, []Edit{e})
	if !strings.Contains(out, `Tom &amp; Jerry &lt;3 "cold brew"`) {
		t.Fatalf("text was not escaped:\n%s", out)
	}
}

func TestSetTextRefusesAVoidElement(t *testing.T) {
	img, _ := FindByID(film, "shot")
	if _, err := SetText(img, "words"); err == nil {
		t.Fatal("accepted text into an <img>")
	}
}

func TestSetAttributeRewritesInPlaceAndKeepsOrder(t *testing.T) {
	el, _ := FindByID(film, "title")
	e, err := SetAttribute(el, film, "style", "color:#054463;font-size:96px")
	if err != nil {
		t.Fatal(err)
	}
	out, _ := Apply(film, []Edit{e})
	if !strings.Contains(out, `<h1 id="title" class="clip" data-start="0" data-duration="3" style="color:#054463;font-size:96px">`) {
		t.Fatalf("attribute order or spelling changed:\n%s", out)
	}
}

func TestSetAttributeAddsOneThatIsNotThere(t *testing.T) {
	el, _ := FindByID(film, "sub")
	e, _ := SetAttribute(el, film, "data-start", "3")
	out, _ := Apply(film, []Edit{e})
	if !strings.Contains(out, `<p data-start="3" id="sub"`) {
		t.Fatalf("new attribute not added after the tag name:\n%s", out)
	}
}

// The case a regex gets wrong: an attribute whose VALUE contains the name
// of the attribute being edited.
func TestSetAttributeIsNotFooledByAValueThatLooksLikeAnAttribute(t *testing.T) {
	el, ok := FindByID(film, "sub")
	if !ok {
		t.Fatal("no #sub")
	}
	e, err := SetAttribute(el, film, "style", "color:red")
	if err != nil {
		t.Fatal(err)
	}
	out, err := Apply(film, []Edit{e})
	if err != nil {
		t.Fatal(err)
	}
	// The decoy inside data-note is untouched…
	if !strings.Contains(out, `data-note="style=&quot;color:red&quot; is text, not markup"`) {
		t.Fatalf("the decoy attribute value was edited:\n%s", out)
	}
	// …and a real style attribute was added.
	if !strings.Contains(out, `<p style="color:red" id="sub"`) {
		t.Fatalf("style was not added to <p>:\n%s", out)
	}
}

func TestSetAttributeEscapesAQuoteInTheValue(t *testing.T) {
	el, _ := FindByID(film, "title")
	e, _ := SetAttribute(el, film, "style", `font-family:"Space Grotesk"`)
	out, _ := Apply(film, []Edit{e})
	if !strings.Contains(out, `style="font-family:&quot;Space Grotesk&quot;"`) {
		t.Fatalf("a quote escaped the attribute:\n%s", out)
	}
	// Which means the document still parses to the same element.
	el2, ok := FindByID(out, "title")
	if !ok || el2.TagName != "h1" {
		t.Fatal("the document stopped parsing after the edit")
	}
}

func TestApplyRunsSeveralEditsAtOnce(t *testing.T) {
	el, _ := FindByID(film, "title")
	text, _ := SetText(el, "Tired of sugar?")
	style, _ := SetAttribute(el, film, "style", "color:#054463")
	start, _ := SetAttribute(el, film, "data-start", "1.5")
	out, err := Apply(film, []Edit{text, style, start})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `data-start="1.5"`) ||
		!strings.Contains(out, `style="color:#054463"`) ||
		!strings.Contains(out, `>Tired of sugar?</h1>`) {
		t.Fatalf("not all three edits landed:\n%s", out)
	}
}

func TestApplyRefusesOverlappingEdits(t *testing.T) {
	_, err := Apply("<p>hello</p>", []Edit{{Start: 0, End: 5, Text: "a"}, {Start: 3, End: 8, Text: "b"}})
	if err == nil {
		t.Fatal("accepted two edits over the same bytes")
	}
}

func TestApplyRefusesAnEditOutsideTheDocument(t *testing.T) {
	if _, err := Apply("<p>hi</p>", []Edit{{Start: 2, End: 999, Text: "x"}}); err == nil {
		t.Fatal("accepted an edit past the end")
	}
}

// A model writes imperfect HTML. An unclosed tag must not make every
// element after it unaddressable.
func TestAnUnclosedTagDoesNotLoseTheElementsAfterIt(t *testing.T) {
	src := `<div id="a"><span id="b">x<div id="c">y</div></div><h1 id="d">z</h1>`
	for _, id := range []string{"a", "b", "c", "d"} {
		if _, ok := FindByID(src, id); !ok {
			t.Fatalf("lost #%s", id)
		}
	}
	d, _ := FindByID(src, "d")
	e, err := SetText(d, "still editable")
	if err != nil {
		t.Fatal(err)
	}
	out, _ := Apply(src, []Edit{e})
	if !strings.Contains(out, `<h1 id="d">still editable</h1>`) {
		t.Fatalf("could not edit past the unclosed tag:\n%s", out)
	}
}

// Nothing is re-serialised, so a document with no edits comes back byte for
// byte — including the doctype, the comments and the whitespace.
func TestNoEditsMeansNoChange(t *testing.T) {
	out, err := Apply(film, nil)
	if err != nil || out != film {
		t.Fatal("an empty edit list changed the document")
	}
}

// The constraint that cost a debugging session: the predicate runs at the
// START TAG, so End is not known yet. A predicate that asks "does this
// contain that?" matches nothing, which reads exactly like "there is no
// such element".
func TestThePredicateSeesTheStartTagOnly(t *testing.T) {
	src := `<section id="s1" data-scene="hook"><h1 id="title">A</h1></section>`
	var endAtPredicateTime int
	FindAll(src, func(e Element) bool {
		if e.Attrs["id"] == "s1" {
			endAtPredicateTime = e.End
		}
		return false
	})
	// At predicate time, End is still the end of the START TAG…
	if endAtPredicateTime != strings.Index(src, "<h1") {
		t.Fatalf("End was %d at predicate time", endAtPredicateTime)
	}
	// …and by the time the element is returned, it covers the whole thing.
	el, ok := FindByID(src, "s1")
	if !ok || el.End != len(src) {
		t.Fatalf("End is %d after the walk, want %d", el.End, len(src))
	}
}
