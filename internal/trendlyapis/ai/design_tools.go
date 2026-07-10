package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// The AI-Studio design tools make the chat AI the editor by having it author
// HTML/CSS — which LLMs do exceptionally well — instead of layout JSON. The app
// renders the HTML in a WebView (WYSIWYG) and captures it to PNG on approve.
//
// generate_design → a fresh self-contained HTML document.
// apply_design_edits → the same document, revised from the user's comments.

const (
	toolGenerateDesign   = "generate_design"
	toolApplyDesignEdits = "apply_design_edits"
)

// moduleHasStudio reports whether a module's chat may drive the design editor.
func moduleHasStudio(module string) bool {
	return module == moduleContent
}

func designServerTools() []openrouter.Tool {
	return []openrouter.Tool{
		openrouter.NewFunctionTool(
			toolGenerateDesign,
			"Design an on-brand social post as a SINGLE self-contained HTML document "+
				"(inline CSS only, no external files or scripts). This is the preferred way "+
				"to create a polished image/carousel/story post.\n"+
				"STRUCTURE (required): wrap everything in <div data-carousel style=\"display:flex\">. "+
				"Inside it put EXACTLY `slides` sibling slides, each "+
				"<section data-slide=\"N\" style=\"flex:0 0 {width}px;width:{width}px;height:{height}px;position:relative;overflow:hidden\"> "+
				"(N = 0,1,2…). A single post is just slides=1. Each slide is a self-contained "+
				"width×height frame.\n"+
				"RULES: keep all text WELL INSIDE each slide (size fonts to fit, never overflow); "+
				"give every editable text element a data-el=\"<unique-id>\" attribute; use the brand "+
				"colors/font from context. For a carousel, tell a story across slides (hook → points "+
				"→ CTA).\n"+
				"VIDEO (docType=video): produce a SINGLE full-frame stage (slides=1, one data-slide=\"0\") "+
				"that ANIMATES using CSS @keyframes over `durationMs` (default ~6000ms): elements fade/"+
				"slide/pop in over time to tell the story as one motion piece. Use animation-fill-mode:"+
				"both and stagger animation-delay so the flow reads start→finish. Do NOT make video a "+
				"multi-slide carousel.\n"+
				"RENDER-SAFE CSS (the design is rasterized to PNG with html2canvas — unsupported CSS "+
				"renders garbled): use ONLY solid text colors — NEVER gradient text / "+
				"background-clip:text / -webkit-text-fill-color:transparent (use a solid brand color "+
				"for accent words instead). NO backdrop-filter, NO mix-blend-mode, NO CSS filter on "+
				"text, NO position:sticky, NO external @import web fonts (use a system font stack like "+
				"font-family:'Helvetica Neue',Arial,sans-serif, or a bold weight). Gradients/solid "+
				"colors as element BACKGROUNDS are fine; box-shadow is fine. Set each slide's "+
				"background explicitly (don't rely on transparency).\n"+
				"Return the full HTML in `html`.",
			openrouter.ObjectSchema(map[string]any{
				"html":    openrouter.StringProp("The complete self-contained HTML document (a data-carousel with `slides` data-slide sections)."),
				"docType": openrouter.EnumProp("image or video.", []string{"image", "video"}),
				"format":  openrouter.EnumProp("Content format.", []string{"post", "reel", "story", "video", "carousel"}),
				"width":      openrouter.NumberProp("Per-slide width in px (e.g. 1080)."),
				"height":     openrouter.NumberProp("Per-slide height in px (e.g. 1350)."),
				"slides":     openrouter.NumberProp("Number of slides (1 for a single post/video; 2-10 for a carousel)."),
				"durationMs": openrouter.NumberProp("For video: total animation length in ms (e.g. 6000). 0 for images."),
			}, []string{"html"}),
		),
		openrouter.NewFunctionTool(
			toolApplyDesignEdits,
			"Revise the current design's HTML to apply the user's pinned comments/"+
				"instructions. You are given the current HTML in context; return the FULL "+
				"revised HTML in `html`, preserving data-el ids and keeping all text inside "+
				"the frame. Make only the requested changes.",
			openrouter.ObjectSchema(map[string]any{
				"html": openrouter.StringProp("The full revised HTML document."),
			}, []string{"html"}),
		),
	}
}

type generateDesignArgs struct {
	HTML       string `json:"html"`
	DocType    string `json:"docType"`
	Format     string `json:"format"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	Slides     int    `json:"slides"`
	DurationMs int    `json:"durationMs"`
}

// countSlides returns the number of data-slide sections in the HTML (min 1).
func countSlides(html string) int {
	n := strings.Count(html, "data-slide")
	if n < 1 {
		return 1
	}
	return n
}

func runGenerateDesign(ctx context.Context, brandID, contentID, arguments string) (string, error) {
	if contentID == "" {
		return jsonResult(map[string]any{"ok": false, "error": "no content in context"}), nil
	}
	var a generateDesignArgs
	if err := json.Unmarshal([]byte(arguments), &a); err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "could not parse arguments"}), nil
	}
	html := strings.TrimSpace(a.HTML)
	if html == "" {
		return jsonResult(map[string]any{"ok": false, "error": "html is required"}), nil
	}
	w, h := a.Width, a.Height
	if w == 0 || h == 0 {
		w, h = sizeForFormatPx(a.Format)
	}
	docType := a.DocType
	if docType == "" {
		docType = "image"
	}
	slides := a.Slides
	if slides < 1 {
		slides = countSlides(html)
	}
	duration := a.DurationMs
	if docType == "video" && duration <= 0 {
		duration = 6000
	}
	revID, err := persistDesign(brandID, contentID, wrapHTML(html), w, h, slides, duration, docType, "generate", "")
	if err != nil {
		return jsonResult(map[string]any{"ok": false, "error": err.Error()}), err
	}
	return jsonResult(map[string]any{
		"ok":         true,
		"revisionId": revID,
		"note":       "design created and shown to the user; describe it briefly and invite them to edit text or pin comments.",
	}), nil
}

type applyDesignArgs struct {
	HTML string `json:"html"`
}

func runApplyDesignEdits(ctx context.Context, brandID, contentID, arguments string) (string, error) {
	if contentID == "" {
		return jsonResult(map[string]any{"ok": false, "error": "no content in context"}), nil
	}
	content, err := trendlymodels.GetContent(brandID, contentID)
	if err != nil || content.DesignRef == nil {
		return jsonResult(map[string]any{"ok": false, "error": "no design to edit; call generate_design first"}), nil
	}
	cur, err := trendlymodels.GetDesignRevision(brandID, contentID, content.DesignRef.RevisionID)
	if err != nil || cur == nil {
		return jsonResult(map[string]any{"ok": false, "error": "current design not found"}), nil
	}
	var a applyDesignArgs
	if err := json.Unmarshal([]byte(arguments), &a); err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "could not parse arguments"}), nil
	}
	html := strings.TrimSpace(a.HTML)
	if html == "" {
		return jsonResult(map[string]any{"ok": false, "error": "html is required"}), nil
	}
	// Preserve the slide count unless the revised HTML changed it.
	slides := cur.SlideCount
	if s := countSlides(html); s != slides && s >= 1 {
		slides = s
	}
	revID, err := persistDesign(brandID, contentID, wrapHTML(html), cur.Width, cur.Height, slides, cur.DurationMs, cur.DocType, "edit", cur.ID)
	if err != nil {
		return jsonResult(map[string]any{"ok": false, "error": err.Error()}), err
	}
	return jsonResult(map[string]any{
		"ok":         true,
		"revisionId": revID,
		"note":       "edits applied and re-rendered; the user can revert to the previous revision.",
	}), nil
}

// persistDesign writes an immutable HTML revision and points the content at it.
func persistDesign(brandID, contentID, html string, w, h, slides, durationMs int, docType, origin, parentRev string) (string, error) {
	if slides < 1 {
		slides = 1
	}
	revID, err := trendlymodels.CreateDesignRevision(brandID, contentID, &trendlymodels.ContentDesignRevision{
		HTML: html, Width: w, Height: h, SlideCount: slides, DurationMs: durationMs, DocType: docType, Origin: origin, ParentRevisionID: parentRev,
	})
	if err != nil {
		return "", err
	}
	if err := trendlymodels.SetContentDesignRef(brandID, contentID, &trendlymodels.ContentDesignRef{
		RevisionID: revID, DocType: docType, Width: w, Height: h, SlideCount: slides, DurationMs: durationMs,
	}); err != nil {
		return "", err
	}
	return revID, nil
}

func sizeForFormatPx(format string) (int, int) {
	switch format {
	case "reel", "story":
		return 1080, 1920
	case "video":
		return 1920, 1080
	case "post", "carousel":
		return 1080, 1350
	default:
		return 1080, 1080
	}
}

// wrapHTML ensures the design is a full document with a deterministic base
// (box-sizing reset + zero margins) so every render is consistent. If the model
// already returned a full document, it is returned unchanged.
func wrapHTML(html string) string {
	low := strings.ToLower(strings.TrimSpace(html))
	if strings.HasPrefix(low, "<!doctype") || strings.HasPrefix(low, "<html") {
		return html
	}
	return "<!doctype html><html><head><meta charset=\"utf-8\">" +
		"<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">" +
		"<style>*{box-sizing:border-box;margin:0;padding:0}html,body{margin:0;padding:0}</style>" +
		"</head><body>" + html + "</body></html>"
}

// brandKitCSS builds a :root CSS variable block from the brand for the AI to
// reference (surfaced via the system prompt context, not injected here).
func brandKitCSS(brandID string) string {
	logo := ""
	if b, err := loadBrand(brandID); err == nil && b != nil && b.Image != nil {
		logo = *b.Image
	}
	return fmt.Sprintf(":root{--brand-navy:#054463;--brand-accent:#02537D;--brand-ink:#16242E;--brand-logo:url(%q)}", logo)
}
