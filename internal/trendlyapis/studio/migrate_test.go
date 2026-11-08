package studio

import (
	"strings"
	"testing"
)

const oldDesign = `<!doctype html>
<html><head><meta charset="utf-8">
<style>
  :root { --brand: #054463; }
  .t { font-family: Quicksand, sans-serif; color: var(--brand); }
  @keyframes rise { from { opacity: 0 } to { opacity: 1 } }
</style>
</head>
<body>
  <div data-slide="0"><h1 class="t">Shelf life</h1><p>one</p></div>
  <div data-slide="1"><h1 class="t">Second beat</h1><img src="https://cdn/x.jpg"></div>
  <div data-slide="2"><h1 class="t">Third</h1><svg><circle r="4"/></svg></div>
</body></html>`

func TestEachSceneKeepsExactlyOneSlide(t *testing.T) {
	parts, err := SplitDesignIntoScenes(oldDesign)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if len(parts) != 3 {
		t.Fatalf("got %d scenes, want 3", len(parts))
	}
	for i, p := range parts {
		if n := strings.Count(p, "data-slide"); n != 1 {
			t.Errorf("scene %d has %d slides, want 1", i, n)
		}
	}
	// In document order — that is the scene order the user authored.
	if !strings.Contains(parts[0], "Shelf life") ||
		!strings.Contains(parts[1], "Second beat") ||
		!strings.Contains(parts[2], "Third") {
		t.Errorf("scenes are out of document order")
	}
	// And each one has only its own content.
	if strings.Contains(parts[0], "Second beat") {
		t.Errorf("scene 0 kept a sibling's content")
	}
}

// ⚠️ THE REASON THIS CONVERTER REMOVES RATHER THAN EXTRACTS. A slide lifted out
// of its document loses the <head> — the stylesheet, the custom properties, the
// keyframes its own rules depend on — and renders as unstyled text. That would
// only ever happen to designs that already existed, which is the worst possible
// place to find out.
func TestEverySceneKeepsTheDocumentAroundIt(t *testing.T) {
	parts, _ := SplitDesignIntoScenes(oldDesign)
	for i, p := range parts {
		for _, want := range []string{"--brand: #054463", "@keyframes rise", "Quicksand", "charset"} {
			if !strings.Contains(p, want) {
				t.Errorf("scene %d lost %q from the document it came from", i, want)
			}
		}
	}
}

func TestChildrenOfTheKeptSlideSurvive(t *testing.T) {
	// Removing siblings must not disturb the slide being kept — including its
	// nested markup, which a naive string split between data-slide occurrences
	// would truncate.
	parts, _ := SplitDesignIntoScenes(oldDesign)
	if !strings.Contains(parts[1], "https://cdn/x.jpg") {
		t.Errorf("scene 1 lost its image")
	}
	if !strings.Contains(parts[2], "<circle") {
		t.Errorf("scene 2 lost its svg")
	}
}

func TestASingleSlideDesignIsLeftAlone(t *testing.T) {
	// Already one scene's worth of document. A parse/render round trip could
	// only change it, so it is not performed.
	one := `<html><body><div data-slide="0">only</div></body></html>`
	parts, err := SplitDesignIntoScenes(one)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if parts != nil {
		t.Errorf("a one-slide design needs no split, got %d parts", len(parts))
	}
}

func TestADesignWithNoSlidesIsLeftAlone(t *testing.T) {
	parts, err := SplitDesignIntoScenes(`<html><body><p>no slides here</p></body></html>`)
	if err != nil || parts != nil {
		t.Errorf("want no split, got %d parts err=%v", len(parts), err)
	}
}

func TestEmptyInputIsNotAnError(t *testing.T) {
	for _, in := range []string{"", "   ", "\n\t"} {
		if parts, err := SplitDesignIntoScenes(in); err != nil || parts != nil {
			t.Errorf("%q: want nothing, got %d parts err=%v", in, len(parts), err)
		}
	}
}

func TestANestedSlideIsNotTreatedAsItsOwnScene(t *testing.T) {
	// No design means this, and counting the inner one would both invent a
	// scene and then delete it out of its parent.
	nested := `<html><body><div data-slide="0">outer<div data-slide="x">inner</div></div>` +
		`<div data-slide="1">second</div></body></html>`
	parts, _ := SplitDesignIntoScenes(nested)
	if len(parts) != 2 {
		t.Fatalf("got %d scenes, want 2", len(parts))
	}
	if !strings.Contains(parts[0], "inner") {
		t.Errorf("the nested node should have stayed inside its parent slide")
	}
}

func TestScenesSplitTheDesignsLengthEvenly(t *testing.T) {
	// The old shape stored ONE length for the whole document and nothing per
	// slide, so an even split is the only number available. It is a guess about
	// timing, which the rail shows to scale and the user can drag.
	scenes, err := ScenesFromDesign(oldDesign, 9000, "reel")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if len(scenes) != 3 {
		t.Fatalf("got %d scenes, want 3", len(scenes))
	}
	for i, sc := range scenes {
		if sc.DurationMs != 3000 {
			t.Errorf("scene %d is %dms, want 3000", i, sc.DurationMs)
		}
		if sc.Index != i {
			t.Errorf("scene %d has index %d", i, sc.Index)
		}
		if len(sc.Layers) != 1 || sc.Layers[0].Kind != "html" || sc.Layers[0].Slot != "full" {
			t.Errorf("scene %d should be one full-frame html layer, got %+v", i, sc.Layers)
		}
		if sc.Layers[0].HTML == "" {
			t.Errorf("scene %d has no html", i)
		}
	}
}

func TestADesignWithNoMeasuredLengthFallsBackToTheFormatDefault(t *testing.T) {
	// A length of zero divided by three is zero, and a scene with no length is
	// refused by the renderer — so the format's own default stands in.
	scenes, err := ScenesFromDesign(oldDesign, 0, "reel")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	for i, sc := range scenes {
		if sc.DurationMs <= 0 {
			t.Errorf("scene %d has no length", i)
		}
	}
}

func TestASingleSlideDesignStillBecomesOneScene(t *testing.T) {
	// SplitDesignIntoScenes returns nothing for one slide; the conversion still
	// has to produce a scene, or opening a studio on a one-slide design would
	// show an empty rail.
	scenes, err := ScenesFromDesign(`<html><body><div data-slide="0">only</div></body></html>`, 4000, "reel")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if len(scenes) != 1 {
		t.Fatalf("got %d scenes, want 1", len(scenes))
	}
	if scenes[0].DurationMs != 4000 {
		t.Errorf("one scene should take the whole length, got %d", scenes[0].DurationMs)
	}
}
