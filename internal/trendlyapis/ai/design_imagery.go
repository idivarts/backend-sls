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
		node   *html.Node
		src    string
		alt    string
		credit string
		stock  bool
	}
	out := make([]filled, len(targets))

	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Add(1)
		go func(i int, t placeholder) {
			defer wg.Done()
			f := filled{node: t.node, stock: t.stock}
			if t.stock {
				f.src, f.alt, f.credit = resolveStock(ctx, brandID, t.prompt, aspect)
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
				res.Credits = append(res.Credits, f.credit)
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

// resolveStock searches Pexels and copies the chosen photo to our own storage.
//
// Copying is not optional. A Pexels URL is stable today, but a design is a
// document the user re-renders weeks later, and pointing it at someone else's
// CDN makes every past design hostage to that host. Same rule as generated
// images.
func resolveStock(ctx context.Context, brandID, query, aspect string) (src, alt, credit string) {
	res, err := mediagen.Submit(ctx, "pexels", mediagen.Spec{
		Kind:        mediagen.KindStock,
		Prompt:      query,
		AspectRatio: aspect,
		Count:       1,
	})
	if err != nil {
		log.Printf("design imagery: stock search %q: %v", query, err)
		return "", "", ""
	}
	if res.Status == mediagen.StatusFailed || len(res.Assets) == 0 {
		// Includes "not configured", which is an ops gap and not worth a log
		// line on every placeholder of every design.
		return "", "", ""
	}
	p := res.Assets[0]
	hosted, uerr := hostAsset(p.URL, brandID)
	if uerr != nil {
		log.Printf("design imagery: stock copy %q: %v", query, uerr)
		return "", "", ""
	}
	return hosted, "", p.Attribution
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
