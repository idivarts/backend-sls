package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/designpreview"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// The AI-Studio design tools make the chat AI the editor by having it author
// HTML/CSS — which LLMs do exceptionally well — instead of layout JSON. The app
// renders the HTML in a WebView (WYSIWYG) and captures it to PNG on approve.
//
// generate_design → a fresh self-contained HTML document.
// apply_design_edits → the same document, revised from the user's comments.

const (
	toolGenerateDesign      = "generate_design"
	toolApplyDesignEdits    = "apply_design_edits"
	toolGetDesignHTML       = "get_design_html"
	toolRenderDesignPreview = "render_design_preview"
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
				"VIDEO (docType=video): build it as a SEQUENCE OF SCENES, exactly like a carousel — "+
				"one [data-carousel] wrapping N [data-slide] SCENES, each a COMPLETE standalone "+
				"full-frame layout (do NOT stack all scenes' content into one frame — that overlaps). "+
				"Set `slides` = number of scenes.\n"+
				"SCENE TRANSITIONS: by default one scene cuts straight to the next. To join two "+
				"scenes instead, give the LATER one data-transition=\"fade\" (or \"push\" or "+
				"\"slide\") and data-transition-ms=\"400\". That scene then starts EARLY and overlaps "+
				"the one before it for that long, and the two are composited:\n"+
				"- fade: the incoming scene cross-dissolves over the outgoing one. The safe default.\n"+
				"- push: both scenes travel together, the outgoing one pushed off as the new one "+
				"arrives. Good for a change of subject.\n"+
				"- slide: the incoming scene slides over a stationary outgoing one.\n"+
				"Use 300-500ms. An overlap is capped at HALF the shorter of the two scenes, so a "+
				"transition longer than that is silently shortened. Leave the FIRST scene with no "+
				"transition — it has nothing to enter from. Cutting is a legitimate choice: cut on a "+
				"beat or a hard change of idea, and transition when the scenes are continuous.\n"+
				"Because overlaps are subtracted, `durationMs` = sum of scene durations MINUS the "+
				"sum of the transition lengths.\n"+
				"⚠️ NEVER set transform or opacity inline on a <section data-slide> itself — the "+
				"player owns those two properties on the slide element to composite transitions, and "+
				"it will overwrite yours. Animate elements INSIDE the scene instead.\n"+
				"THE RENDERER: the design is screenshotted by real Chromium, server-side, one element "+
				"capture per [data-slide]. Modern CSS is fully supported and you should use it — "+
				"gradient text (background-clip:text), backdrop-filter, mix-blend-mode, CSS filters, "+
				"box-shadow, real web fonts, transforms and 3D. Prefer the brand font from the brand "+
				"kit above over a system stack. Flat, system-font output is a wasted render. Give each "+
				"slide an explicit background (don't rely on transparency), and size the slide itself — "+
				"never rely on the viewport (vh/vw), since each slide is captured as an element.\n"+
				"MOTION (the one place the renderer is strict): every frame is produced by pausing all "+
				"animations and setting currentTime, so ONLY CSS @keyframes animations render. CSS "+
				"`transition`, JS-driven animation and the Web Animations API DO NOT RENDER AT ALL — "+
				"they are silently dropped. Rules:\n"+
				"- Animate transform and opacity ONLY. Other properties jank and may not seek cleanly.\n"+
				"- EVERY scene needs an explicit data-duration=\"<ms>\" (3000-5000 is typical). A scene "+
				"without one silently falls back to 3000ms, which desynchronises the whole video.\n"+
				"- Motion must SETTLE well inside its own scene: animation-delay + animation-duration "+
				"must be comfortably less than that scene's data-duration, and use "+
				"animation-fill-mode:both so elements hold their final state for the rest of the scene. "+
				"Never animation-iteration-count:infinite — there is no stable frame to capture.\n"+
				"- Set transform-origin deliberately on anything that scales or rotates.\n"+
				"- Stagger siblings by 60-80ms rather than entering everything at once, and use real "+
				"easing (a cubic-bezier with a little overshoot) rather than the default `ease`.\n"+
				"PHOTOGRAPHY: flat gradients and type are what make a design look generated. "+
				"Where a photograph would carry the piece — a background behind a headline, a "+
				"product shot, a human face — do NOT draw it in CSS and do NOT invent a URL. "+
				"Write a placeholder and the server fills it in before the user ever sees the "+
				"document:\n"+
				"- `<img data-stock=\"<description>\" data-el=\"bg\">` — real photography, searched "+
				"from a stock library. Use this for anything that should look like a real "+
				"photograph of a real place, object or person.\n"+
				"- `<img data-gen=\"<description>\" data-el=\"hero\">` — a generated image. Use this "+
				"only when the shot has to be specific in a way stock cannot be (a particular "+
				"product, an imagined scene).\n"+
				"Describe the SHOT, not the mood: subject, framing, light, and any space the "+
				"layout needs left empty (\"wide shot of an empty café at dawn, lots of flat wall "+
				"on the left for a headline\"). Give every placeholder a data-el, size it with CSS "+
				"as you would any image (object-fit:cover and an explicit width/height), and put "+
				"a scrim behind text that sits over it so the type still reads. Prefer ONE strong "+
				"photograph per scene over several small ones. A placeholder that cannot be "+
				"filled renders as nothing, so never make one load-bearing for the layout.\n"+
				"Do NOT write a photo credit yourself. A stock photo's licence requires one, and "+
				"the server adds it to the scene automatically in the bottom-left corner — so "+
				"leave that corner clear of anything you would mind it sitting over, and never "+
				"invent a photographer's name.\n"+
				"Return the full HTML in `html`.",
			openrouter.ObjectSchema(map[string]any{
				"html":       openrouter.StringProp("The complete self-contained HTML document (a data-carousel with `slides` data-slide sections)."),
				"docType":    openrouter.EnumProp("image or video.", []string{"image", "video"}),
				"format":     openrouter.EnumProp("Content format.", []string{"post", "reel", "story", "video", "carousel"}),
				"width":      openrouter.NumberProp("Per-slide width in px (e.g. 1080)."),
				"height":     openrouter.NumberProp("Per-slide height in px (e.g. 1350)."),
				"slides":     openrouter.NumberProp("Number of slides (1 for a single post/video; 2-10 for a carousel)."),
				"durationMs": openrouter.NumberProp("For video: total animation length in ms (e.g. 6000). 0 for images."),
			}, []string{"html"}),
		),
		openrouter.NewFunctionTool(
			toolApplyDesignEdits,
			"Revise the current design. The current HTML is NOT in your context — call "+
				"get_design_html FIRST, then edit it.\n"+
				"PREFER `edits`: a list of {oldText, newText} replacements applied in order. "+
				"Each oldText must appear EXACTLY ONCE in the document — if a snippet repeats "+
				"(a shared style, a repeated class), include enough surrounding text to make it "+
				"unique. Send every change you need in ONE call; they are applied together or "+
				"not at all.\n"+
				"Snippet edits are strongly preferred over resending the document: re-typing a "+
				"multi-scene design to change one word is how elements get silently dropped and "+
				"keyframes get mangled. Only use `html` (a full replacement) when the design "+
				"changes throughout — a new structure, a different scene count.\n"+
				"Either way, preserve data-el ids (the user's comments are anchored to them) and "+
				"keep every rule the design contract states. After this returns, call "+
				toolRenderDesignPreview+" and look at the result.",
			openrouter.ObjectSchema(map[string]any{
				"edits": map[string]any{
					"type": "array",
					"description": "Targeted replacements, applied in order. Preferred over `html`. " +
						"Each oldText must match the current document exactly and occur exactly once.",
					"items": openrouter.ObjectSchema(map[string]any{
						"oldText": openrouter.StringProp("The exact existing text to replace, copied verbatim from get_design_html."),
						"newText": openrouter.StringProp("What to put in its place. An empty string deletes the snippet."),
					}, []string{"oldText", "newText"}),
				},
				"html": openrouter.StringProp(
					"A FULL replacement document. Use only when the design changes throughout; " +
						"prefer `edits` for anything smaller.",
				),
			}, []string{}),
		),
		openrouter.NewFunctionTool(
			toolRenderDesignPreview,
			"LOOK at the current design: renders it in a real browser and shows you the "+
				"actual frames as images. You author HTML blind, so this is the only way to "+
				"catch what text alone hides — text overflowing or clipped at a slide edge, "+
				"elements colliding or overlapping, a layout that is optically off-centre, or "+
				"(for video) motion that has not arrived yet or finished long before its scene "+
				"ends.\n"+
				"ALWAYS call this right after generate_design and after every apply_design_edits. "+
				"Then look at what comes back and judge it honestly: if anything is wrong, call "+
				"apply_design_edits to fix it and render again. Two or three rounds is normal and "+
				"expected — do not tell the user the design is ready until you have seen a frame "+
				"you would be happy to publish.\n"+
				"By default it samples every slide of a still design, and three moments inside "+
				"each scene of a video. Pass `frames` only to look somewhere specific.",
			openrouter.ObjectSchema(map[string]any{
				"frames": map[string]any{
					"type": "array",
					"description": "Optional. Specific frames to capture. Each is " +
						"{slide:<index from 0>, ms:<time into that scene>}. A NEGATIVE ms is a " +
						"fraction of that scene's duration (-0.15 = 15% in), which is how you " +
						"look just after an entrance without knowing the exact timing.",
					"items": openrouter.ObjectSchema(map[string]any{
						"slide": openrouter.NumberProp("Slide/scene index, starting at 0."),
						"ms":    openrouter.NumberProp("Time into that scene in ms, or a negative fraction of its duration."),
					}, []string{"slide", "ms"}),
				},
			}, []string{}),
		),
		openrouter.NewFunctionTool(
			toolGetDesignHTML,
			"Fetch the FULL, current HTML of this content's design (never truncated). The "+
				"design HTML is deliberately NOT in your context (it is large) — only a "+
				"reference is. So you MUST call this FIRST to read or edit an existing design: "+
				"run it whenever the user asks about or wants to change the current design, "+
				"then use the returned `html` with apply_design_edits. Returns "+
				"{ html, revisionId, docType, slideCount, width, height, durationMs }.",
			openrouter.ObjectSchema(map[string]any{
				"revisionId": openrouter.StringProp(
					"Optional. A specific design revision id to fetch; omit to get the content's current design.",
				),
			}, []string{}),
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
	// Gate before persisting: a design that cannot survive the renderer should
	// never become a revision the user can see or publish. The model gets the
	// specific failures back and fixes them on the next step.
	if problems := validateDesignHTML(html, docType, a.Slides); len(problems) > 0 {
		return designProblemsResult(problems), nil
	}
	// Resolve every data-gen / data-stock placeholder before the document
	// becomes a revision, so the design the user sees already has its
	// photography in it. Runs AFTER validation — there is no point spending
	// money on images for a document that will not render.
	filled, imagery := fillDesignImagery(ctx, brandID, aspectForSize(w, h), wrapHTML(html))

	revID, err := persistDesign(brandID, contentID, filled, w, h, slides, duration, docType, "generate", "")
	if err != nil {
		return jsonResult(map[string]any{"ok": false, "error": err.Error()}), err
	}
	out := map[string]any{
		"ok":         true,
		"revisionId": revID,
		"note":       "design created and shown to the user; describe it briefly and invite them to edit text or pin comments.",
	}
	if imagery.touched() {
		out["imagery"] = map[string]any{
			"generated": imagery.Generated,
			"stock":     imagery.Stock,
			"failed":    imagery.Failed,
			"credits":   imagery.Credits,
		}
		if imagery.Failed > 0 {
			// Say so plainly. An <img> with no src is an invisible gap, and the
			// model is the only thing that can tell the user a photo is missing
			// or offer to try a different description.
			out["note"] = "design created. " + fmt.Sprint(imagery.Failed) +
				" image placeholder(s) could not be filled and are empty — tell the user which, " +
				"and offer to try a different description or a different source."
		}
	}
	return jsonResult(out), nil
}

// aspectForSize names the closest ratio for a pixel size, so stock search and
// image generation are asked for the shape the slide actually is.
func aspectForSize(w, h int) string {
	if w <= 0 || h <= 0 {
		return "1:1"
	}
	r := float64(w) / float64(h)
	switch {
	case r < 0.65:
		return "9:16"
	case r < 0.95:
		return "4:5"
	case r < 1.2:
		return "1:1"
	default:
		return "16:9"
	}
}

// designEdit is one surgical replacement: swap an exact snippet for another.
type designEdit struct {
	OldText string `json:"oldText"`
	NewText string `json:"newText"`
}

type applyDesignArgs struct {
	Edits []designEdit `json:"edits"`
	HTML  string       `json:"html"`
}

// applySnippetEdits applies edits in order and returns the revised document.
//
// Each anchor must match EXACTLY ONCE. Matching the first of several is how an
// edit meant for scene 4 silently lands on scene 1 — a shared style string or a
// repeated class is exactly the kind of text a model picks as an anchor — so an
// ambiguous anchor is refused and the model is told to quote more context.
//
// Edits apply to the result of the previous one, so a later anchor may be text
// an earlier edit introduced. Failure is all-or-nothing: the caller gets an
// error and the stored design is untouched.
func applySnippetEdits(doc string, edits []designEdit) (string, error) {
	for i, e := range edits {
		if e.OldText == "" {
			return "", fmt.Errorf("edit %d has an empty oldText", i)
		}
		switch n := strings.Count(doc, e.OldText); {
		case n == 0:
			return "", fmt.Errorf(
				"edit %d: oldText was not found verbatim in the current design. "+
					"Call %s again and copy the text exactly as it stands",
				i, toolGetDesignHTML)
		case n > 1:
			return "", fmt.Errorf(
				"edit %d: oldText appears %d times, so it is ambiguous. Include more "+
					"surrounding text so it matches exactly once", i, n)
		}
		doc = strings.Replace(doc, e.OldText, e.NewText, 1)
	}
	return doc, nil
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
	// Two ways in, and the surgical one is the default: `edits` patches the
	// stored document in place, so everything the model did not name survives
	// byte for byte. `html` stays as the escape hatch for a design that really
	// does change throughout.
	// Both land as origin "edit": that field is a shared enum the brand app's
	// types mirror ("generate" | "edit" | "text" | "revert"), and splitting a
	// "rewrite" out of it would break the frontend union for no user-visible gain.
	var html string
	switch {
	case len(a.Edits) > 0:
		patched, err := applySnippetEdits(cur.HTML, a.Edits)
		if err != nil {
			// Nothing was written — the model fixes its anchor and retries.
			return jsonResult(map[string]any{"ok": false, "error": err.Error()}), nil
		}
		html = patched
	case strings.TrimSpace(a.HTML) != "":
		html = strings.TrimSpace(a.HTML)
	default:
		return jsonResult(map[string]any{
			"ok": false,
			"error": "nothing to apply: send `edits` (preferred) or a full `html` " +
				"replacement",
		}), nil
	}
	// Preserve the slide count unless the revised HTML changed it.
	slides := cur.SlideCount
	if s := countSlides(html); s != slides && s >= 1 {
		slides = s
	}
	// An edit is gated exactly like a generate. Passing 0 for wantSlides because
	// an edit is allowed to change the slide count deliberately.
	if problems := validateDesignHTML(html, cur.DocType, 0); len(problems) > 0 {
		return designProblemsResult(problems), nil
	}
	revID, err := persistDesign(brandID, contentID, wrapHTML(html), cur.Width, cur.Height, slides, cur.DurationMs, cur.DocType, "edit", cur.ID)
	if err != nil {
		return jsonResult(map[string]any{"ok": false, "error": err.Error()}), err
	}
	return jsonResult(map[string]any{
		"ok":           true,
		"revisionId":   revID,
		"editsApplied": len(a.Edits),
		"note": "edits applied; the user can revert to the previous revision. Call " +
			toolRenderDesignPreview + " now and check the result.",
	}), nil
}

type getDesignHTMLArgs struct {
	RevisionID string `json:"revisionId"`
}

// runGetDesignHTML returns the FULL current design HTML on demand. The design
// HTML is intentionally kept OUT of the system context (it is large and would
// truncate the prompt); the model fetches the complete document here right
// before reading or editing it. Not truncated — the model needs the whole thing
// to return a faithful revised document via apply_design_edits.
func runGetDesignHTML(ctx context.Context, brandID, contentID, arguments string) (string, error) {
	if contentID == "" {
		return jsonResult(map[string]any{"ok": false, "error": "no content in context"}), nil
	}
	content, err := trendlymodels.GetContent(brandID, contentID)
	if err != nil || content == nil || content.DesignRef == nil || content.DesignRef.RevisionID == "" {
		return jsonResult(map[string]any{"ok": false, "error": "no design yet; call generate_design first"}), nil
	}
	var a getDesignHTMLArgs
	_ = json.Unmarshal([]byte(arguments), &a)
	revID := strings.TrimSpace(a.RevisionID)
	if revID == "" {
		revID = content.DesignRef.RevisionID
	}
	rev, err := trendlymodels.GetDesignRevision(brandID, contentID, revID)
	if err != nil || rev == nil {
		return jsonResult(map[string]any{"ok": false, "error": "design revision not found"}), nil
	}
	return jsonResult(map[string]any{
		"ok":         true,
		"revisionId": rev.ID,
		"docType":    rev.DocType,
		"slideCount": rev.SlideCount,
		"width":      rev.Width,
		"height":     rev.Height,
		"durationMs": rev.DurationMs,
		"html":       rev.HTML,
	}), nil
}

type renderPreviewArgs struct {
	Frames []designpreview.Frame `json:"frames"`
}

// previewMaxEdge is the longest edge of an inspection frame, in px. Big enough
// to read a headline and see a collision, small enough that three rounds of
// looking don't dominate the turn's token bill.
const previewMaxEdge = 640

// runRenderDesignPreview renders the current design and hands the frames back
// for the model to look at.
//
// It returns the images SEPARATELY from the JSON result because a tool message
// carries text only — the frames have to ride back on a following user turn as
// multimodal content (see handleMessageWS). The JSON result is what the model
// reads as the tool's answer; the images are what it actually looks at.
func runRenderDesignPreview(ctx context.Context, brandID, contentID, arguments string) (string, []string, error) {
	if contentID == "" {
		return jsonResult(map[string]any{"ok": false, "error": "no content in context"}), nil, nil
	}
	content, err := trendlymodels.GetContent(brandID, contentID)
	if err != nil || content == nil || content.DesignRef == nil || content.DesignRef.RevisionID == "" {
		return jsonResult(map[string]any{"ok": false, "error": "no design yet; call generate_design first"}), nil, nil
	}
	var a renderPreviewArgs
	_ = json.Unmarshal([]byte(arguments), &a)

	res, err := designpreview.Render(brandID, contentID, content.DesignRef.RevisionID, a.Frames, previewMaxEdge)
	if err != nil {
		// Not being able to look is a degraded turn, not a failed one: say so
		// plainly so the model carries on rather than retrying forever.
		if errors.Is(err, designpreview.ErrNotConfigured) {
			return jsonResult(map[string]any{
				"ok":    false,
				"error": "design preview is not available in this environment; continue without looking at the render",
			}), nil, nil
		}
		return jsonResult(map[string]any{"ok": false, "error": err.Error()}), nil, nil
	}

	labels := make([]map[string]any, 0, len(res.Captured))
	for _, f := range res.Captured {
		labels = append(labels, map[string]any{"slide": f.Slide, "ms": f.Ms})
	}
	return jsonResult(map[string]any{
		"ok":         true,
		"docType":    res.DocType,
		"slideCount": res.SlideCount,
		"width":      res.Width,
		"height":     res.Height,
		"frames":     labels,
		"note": "the rendered frames follow as images, in this order. Look at them and " +
			"judge the design honestly: check that no text overflows or is clipped, that " +
			"nothing collides, that the layout is balanced, and (for video) that each scene's " +
			"motion has landed by mid-scene. Fix anything wrong with apply_design_edits, then " +
			"render again.",
	}), res.Images, nil
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
