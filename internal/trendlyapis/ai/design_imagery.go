package ai

import (
	"context"
	"log"
	"strings"
	"sync"

	"github.com/idivarts/backend-sls/pkg/mediagen"
	"github.com/idivarts/backend-sls/pkg/openrouter"
	"golang.org/x/net/html"
)

// Placeholder-then-fill: how real photography gets into an AI-authored design.
//
// The model writes ONE self-contained document and marks every image it wants
// as a placeholder:
//
//	<img data-gen="cozy cafe interior, warm morning light" data-el="hero">
//	<img data-stock="overhead shot of a laptop on oak" data-el="bg">
//
// and this pass resolves them all in parallel afterwards.
//
// The alternative — letting the model call an image tool mid-authoring — was
// rejected: it already returns a ~50KB document in one shot, and interleaving
// tool calls with that in flight is both slow and fragile. Resolving afterwards
// also leaves each image INDIVIDUALLY replaceable later ("swap the hero photo")
// without regenerating the document, because each one keeps its own data-el.
//
// Unresolved placeholders are left exactly as they are rather than removed. An
// <img> with no src renders as nothing, which is a visible gap the user can ask
// about — whereas deleting the element silently changes the layout the model
// designed, and nobody can tell what is missing.

const (
	attrGen   = "data-gen"
	attrStock = "data-stock"
)

// imageryResult reports what a fill pass did, for the tool result the model reads.
type imageryResult struct {
	Generated int
	Stock     int
	Failed    int
	// Credits are the attributions that must travel with the design.
	Credits []string
}

func (r imageryResult) touched() bool { return r.Generated+r.Stock+r.Failed > 0 }

// fillDesignImagery resolves every data-gen / data-stock placeholder in `doc`
// and returns the rewritten HTML.
//
// It never fails the caller: an image that cannot be resolved leaves its
// placeholder in place and is counted in Failed. A design with a missing photo
// is worth far more than no design.
func fillDesignImagery(ctx context.Context, brandID, aspect string, doc string) (string, imageryResult) {
	var res imageryResult
	if !strings.Contains(doc, attrGen) && !strings.Contains(doc, attrStock) {
		return doc, res
	}

	root, err := html.Parse(strings.NewReader(doc))
	if err != nil {
		// A document we cannot parse is one we must not rewrite by hand.
		log.Printf("design imagery: parse failed, leaving placeholders: %v", err)
		return doc, res
	}

	targets := collectPlaceholders(root)
	if len(targets) == 0 {
		return doc, res
	}

	type filled struct {
		node      *html.Node
		src       string
		alt       string
		credit    string
		creditURL string
		stock     bool
	}
	out := make([]filled, len(targets))

	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Add(1)
		go func(i int, t placeholder) {
			defer wg.Done()
			f := filled{node: t.node, stock: t.stock}
			if t.stock {
				f.src, f.alt, f.credit, f.creditURL = resolveStock(ctx, brandID, t.prompt, aspect)
			} else {
				f.src = resolveGenerated(ctx, brandID, t.prompt, aspect)
			}
			out[i] = f
		}(i, t)
	}
	wg.Wait()

	for _, f := range out {
		if f.src == "" {
			res.Failed++
			continue
		}
		setAttr(f.node, "src", f.src)
		if f.alt != "" {
			setAttr(f.node, "alt", f.alt)
		}
		if f.stock {
			res.Stock++
			if f.credit != "" {
				setAttr(f.node, "data-credit", f.credit)
				if f.creditURL != "" {
					setAttr(f.node, "data-credit-url", f.creditURL)
				}
				res.Credits = append(res.Credits, f.credit)
				// An attribute nobody paints is not attribution. Unsplash's §9
				// and Guideline 3 require the credit to be DISPLAYED where the
				// photo is, so it goes into the document itself.
				placeCredit(f.node, f.credit, f.creditURL)
			}
		} else {
			res.Generated++
		}
	}

	var b strings.Builder
	if err := html.Render(&b, root); err != nil {
		log.Printf("design imagery: render failed, keeping original: %v", err)
		return doc, imageryResult{}
	}
	return b.String(), res
}

type placeholder struct {
	node   *html.Node
	prompt string
	stock  bool
}

func collectPlaceholders(root *html.Node) []placeholder {
	var found []placeholder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "img" {
			if p := trimmedAttr(n, attrStock); p != "" {
				found = append(found, placeholder{node: n, prompt: p, stock: true})
			} else if p := trimmedAttr(n, attrGen); p != "" {
				found = append(found, placeholder{node: n, prompt: p})
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return found
}

// resolveStock searches the configured stock provider and places one photo.
//
// ── Whether the bytes are copied is the PROVIDER's call, not ours ───────────
// This used to copy unconditionally, and the reasoning was sound: a design is a
// document the user re-renders weeks later, and pointing it at someone else's
// CDN makes every past design hostage to that host. Unsplash's API terms
// (Guideline 1, §6) require an app to use the URLs the API returns and call
// failure to do so a material breach, so for that provider we must accept
// exactly the risk the old comment refused.
//
// Hence Asset.Hotlink: Pexels still gets copied, because its licence allows it
// and durability is worth having wherever it is free. A hotlinked photo is NOT
// a bug, and the render worker fetching images.unsplash.com at capture time is
// the intended behaviour.
//
// One consolation: a hotlinked photo carries its provenance in its own URL,
// which is what makes the "stock may not feed a model" gate enforceable at all.
// A copy in our bucket would be indistinguishable from a user's upload.
func resolveStock(ctx context.Context, brandID, query, aspect string) (src, alt, credit, creditURL string) {
	provider := mediagen.StockProvider()
	res, err := mediagen.Submit(ctx, provider, mediagen.Spec{
		Kind:        mediagen.KindStock,
		Prompt:      query,
		AspectRatio: aspect,
		Count:       1,
	})
	if err != nil {
		log.Printf("design imagery: stock search %q: %v", query, err)
		return "", "", "", ""
	}
	if res.Status == mediagen.StatusFailed || len(res.Assets) == 0 {
		// Includes "not configured", which is an ops gap and not worth a log
		// line on every placeholder of every design.
		return "", "", "", ""
	}

	p := res.Assets[0]
	src = p.URL
	if !p.Hotlink {
		hosted, uerr := hostAsset(p.URL, brandID)
		if uerr != nil {
			log.Printf("design imagery: stock copy %q: %v", query, uerr)
			return "", "", "", ""
		}
		src = hosted
	}

	// The placement event. Some providers require the app to report a photo
	// that is actually USED, as opposed to merely returned by a search — so
	// this fires here, once, for the one photo being placed, and never in the
	// driver where a search of ten results would report ten uses.
	//
	// A failed report does not fail the design: the photo is already placed,
	// and dropping the whole image over an analytics ping would be a far worse
	// trade than an under-reported download.
	if rerr := mediagen.ReportUsed(ctx, provider, p); rerr != nil {
		log.Printf("design imagery: stock usage report %q: %v", query, rerr)
	}

	return src, p.Alt, p.Attribution, p.CreatorURL
}

// resolveGenerated produces an image from the model and hosts it.
func resolveGenerated(ctx context.Context, brandID, prompt, aspect string) string {
	model, locked := pickModel(ctx, brandID, openrouter.TaskImage, "")
	if locked {
		// The plan does not include image generation. Silently leaving the
		// placeholder is right: the design still lands, and the gate is
		// reported by the surfaces that can actually prompt an upgrade.
		return ""
	}
	res, err := mediagen.Submit(ctx, "openrouter", mediagen.Spec{
		Kind:        mediagen.KindImage,
		ModelID:     model,
		Prompt:      designSystemImageryGuidance(loadDesignSystem(brandID), prompt),
		AspectRatio: aspect,
		Count:       1,
	})
	if err != nil || res.Status == mediagen.StatusFailed {
		log.Printf("design imagery: generate %q: %v %s", prompt, err, res.Error)
		return ""
	}
	if res.Usage != nil {
		if orgID, ok := orgIDForBrand(brandID); ok {
			meterAIUsage(orgID, &openrouter.Usage{Cost: res.Usage.CostUSD})
		}
	}
	for _, a := range res.Assets {
		if s3url, uerr := hostAsset(a.URL, brandID); uerr == nil && s3url != "" {
			return s3url
		}
	}
	return ""
}

// trimmedAttr is attr() with the whitespace a hand-written template picks up
// removed, so a placeholder indented across lines still resolves.
func trimmedAttr(n *html.Node, key string) string {
	v, _ := attr(n, key)
	return strings.TrimSpace(v)
}

func setAttr(n *html.Node, key, val string) {
	for i := range n.Attr {
		if n.Attr[i].Key == key {
			n.Attr[i].Val = val
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Key: key, Val: val})
}

// placeCredit writes a visible photo credit onto the slide holding `img`.
//
// Attribution stopped being a courtesy when the stock provider changed: §9 and
// Guideline 3 of Unsplash's terms require the credit to be shown where the
// photo is shown, and to link to the photographer with a referral tag. A
// `data-credit` attribute satisfied neither — nothing in the product ever
// painted it, so in practice the credit did not exist.
//
// It is deliberately small, corner-anchored and low-contrast rather than
// styled to match: this runs on a document the model designed, and a credit
// that tries to look designed is a credit that fights the layout. It carries
// `data-el` so it is addressable like anything else — a user can ask the AI to
// move or restyle it, and the editor can select it.
//
// One credit per slide, not per photo: a slide using two stock photos gets one
// line naming both, because two stacked captions in a corner is worse than one.
func placeCredit(img *html.Node, credit, creditURL string) {
	host := creditHost(img)
	if host == nil {
		return
	}

	// Already credited on this slide — extend the existing line rather than
	// stacking a second one.
	if existing := findCredit(host); existing != nil {
		appendCreditText(existing, credit, creditURL)
		return
	}

	node := &html.Node{
		Type: html.ElementNode,
		Data: "div",
		Attr: []html.Attribute{
			// Unique per slide. A data-el is how the editor selects an element
			// and how a comment anchors to one, so two slides sharing an id
			// would make both uneditable and pin every comment to whichever
			// matched first.
			{Key: "data-el", Val: creditElID(host)},
			{Key: "style", Val: creditStyle},
		},
	}
	appendCreditText(node, credit, creditURL)
	host.AppendChild(node)
}

// creditStyle keeps the credit legible on any photograph without a backdrop:
// a text shadow reads on light and dark alike, where a translucent pill would
// have to guess at the design's corner radius and palette.
const creditStyle = "position:absolute;left:24px;bottom:18px;z-index:9;" +
	"font-size:13px;line-height:1.3;color:rgba(255,255,255,.72);" +
	"text-shadow:0 1px 3px rgba(0,0,0,.55);letter-spacing:.2px;" +
	"font-family:inherit;pointer-events:auto"

// creditHost finds the element the credit should sit in: the slide containing
// the image, or the body for a design with no slides.
//
// It must be POSITIONED for an absolute credit to anchor to it. A [data-slide]
// always is — the design prompt requires `position:relative` on every scene —
// so this does not restyle anything it is given.
func creditHost(img *html.Node) *html.Node {
	for n := img.Parent; n != nil; n = n.Parent {
		if n.Type != html.ElementNode {
			continue
		}
		if _, ok := attr(n, "data-slide"); ok {
			return n
		}
		if n.Data == "body" {
			return n
		}
	}
	return nil
}

// creditElID names the credit after the slide it sits on, so it is unique
// across the document. A design with no slides has only one credit, so the bare
// name is already unique there.
func creditElID(host *html.Node) string {
	if idx := trimmedAttr(host, "data-slide"); idx != "" {
		return "stock-credit-" + idx
	}
	return "stock-credit"
}

func findCredit(host *html.Node) *html.Node {
	want := creditElID(host)
	for c := host.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && trimmedAttr(c, "data-el") == want {
			return c
		}
	}
	return nil
}

// appendCreditText adds one credit to a credit element, as a link when the
// photographer's profile is known and as plain text when it is not.
func appendCreditText(node *html.Node, credit, creditURL string) {
	if node.FirstChild != nil {
		node.AppendChild(&html.Node{Type: html.TextNode, Data: " · "})
	}
	if creditURL == "" {
		node.AppendChild(&html.Node{Type: html.TextNode, Data: credit})
		return
	}
	link := &html.Node{
		Type: html.ElementNode,
		Data: "a",
		Attr: []html.Attribute{
			{Key: "href", Val: creditURL},
			{Key: "target", Val: "_blank"},
			{Key: "rel", Val: "noopener noreferrer"},
			{Key: "style", Val: "color:inherit;text-decoration:underline"},
		},
	}
	link.AppendChild(&html.Node{Type: html.TextNode, Data: credit})
	node.AppendChild(link)
}
