package studio

// Converting an OLD design into studio scenes.
//
// ── What the old shape was, and why it has to change ───────────────────────
// A design was ONE HTML document containing N `[data-slide]` elements. That is
// why the old studio could not reorder a scene, delete one, or regenerate
// scene four: "scene" was a node inside a file, so every structural edit meant
// rewriting the whole document and every regeneration meant regenerating all of
// it. The studio makes a scene a DOCUMENT, which is what buys reorder, split,
// duplicate and per-scene generation.
//
// ── Why the conversion keeps the whole document per scene ──────────────────
// Each scene's html is the original document with the OTHER slides removed —
// not the slide's markup lifted out of it. Lifting would be smaller and would
// lose the `<head>`: the stylesheet, the font links, the keyframes, the CSS
// custom properties the slide's own rules depend on. A slide extracted from its
// document renders as unstyled text, and it would do so only for designs that
// had already been made, which is the worst possible place to discover it.
//
// Removing instead of extracting also means anything we have not thought of —
// a script, a <defs> block, a shared SVG symbol — comes along by construction.
//
// ⚠️ IDEMPOTENT AND LAZY. It runs when a studio is opened for a content that
// has an old design and no scenes yet, which is the only moment it is needed.
// There is no batch job over every brand's designs: a migration that rewrites
// documents nobody is looking at can only be discovered by its damage, whereas
// this one is discovered by the user it just helped.

import (
	"bytes"
	"strings"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"golang.org/x/net/html"
)

// hasSlideAttr reports whether a node carries data-slide.
func hasSlideAttr(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}
	for _, a := range n.Attr {
		if a.Key == "data-slide" {
			return true
		}
	}
	return false
}

// collectSlides walks the tree once and returns the slide elements in document
// order. Order matters: it is the scene order the user authored.
func collectSlides(root *html.Node) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if hasSlideAttr(n) {
			out = append(out, n)
			// Not descending into a slide: a nested [data-slide] inside another
			// would be counted as its own scene and then removed from its
			// parent, which no design means and every design would survive
			// worse for.
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return out
}

// SplitDesignIntoScenes turns one multi-slide design document into one document
// per slide, in document order.
//
// Returns nil when the design has fewer than two slides: a single-slide design
// is already one scene's worth of document and needs no surgery, and copying it
// through a parse/render round trip would only risk changing it.
func SplitDesignIntoScenes(doc string) ([]string, error) {
	if strings.TrimSpace(doc) == "" {
		return nil, nil
	}

	// Parsed once to count. Each scene then gets its OWN parse, because
	// removing a node mutates the tree and a second scene cannot be taken from
	// a tree the first one has already edited.
	first, err := html.Parse(strings.NewReader(doc))
	if err != nil {
		return nil, err
	}
	count := len(collectSlides(first))
	if count < 2 {
		return nil, nil
	}

	out := make([]string, 0, count)
	for keep := 0; keep < count; keep++ {
		tree, err := html.Parse(strings.NewReader(doc))
		if err != nil {
			return nil, err
		}
		slides := collectSlides(tree)
		if len(slides) != count {
			// The two parses disagreed, which should be impossible for the same
			// input. Refusing beats writing scenes built from a tree we cannot
			// reason about.
			return nil, nil
		}
		for i, s := range slides {
			if i == keep || s.Parent == nil {
				continue
			}
			s.Parent.RemoveChild(s)
		}
		var buf bytes.Buffer
		if err := html.Render(&buf, tree); err != nil {
			return nil, err
		}
		out = append(out, buf.String())
	}
	return out, nil
}

// ScenesFromDesign builds the scene documents a studio should start with, from
// an old design revision.
//
// The duration comes from the design's own measured length divided evenly,
// because the old shape stored one length for the whole document and nothing
// per slide. An even split is a guess, and it is the only guess available —
// but it is a guess about TIMING, which the rail shows to scale and the user
// can drag, rather than a guess about content.
func ScenesFromDesign(doc string, totalMs int, format string) ([]trendlymodels.StudioScene, error) {
	parts, err := SplitDesignIntoScenes(doc)
	if err != nil {
		return nil, err
	}
	if len(parts) == 0 {
		// One slide, or none: the whole document is one scene.
		if strings.TrimSpace(doc) == "" {
			return nil, nil
		}
		parts = []string{doc}
	}

	per := 0
	if totalMs > 0 {
		per = totalMs / len(parts)
	}
	if per <= 0 {
		per = defaultSceneMs(format)
	}

	scenes := make([]trendlymodels.StudioScene, 0, len(parts))
	for i, part := range parts {
		scenes = append(scenes, trendlymodels.StudioScene{
			PrimaryKind: trendlymodels.SceneMotion,
			DurationMs:  per,
			Index:       i,
			Layers: []trendlymodels.StudioLayer{{
				ID:   "l1",
				Kind: "html",
				Slot: "full",
				HTML: part,
			}},
		})
	}
	return scenes, nil
}
