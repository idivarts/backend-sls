package design

// Asking the model to change ONE element.
//
// ── What makes this an edit rather than a re-roll ─────────────────────────
// Phase 14: "the AI panel stops being a chat about the film and becomes the
// thing that changes it… the reply is applied as a targeted replacement, so
// an edit cannot silently change a different scene. This is the difference
// between an edit and a re-roll."
//
// So the model is shown one element and its scene for context, and is asked
// for a replacement for that element ALONE. What comes back is spliced over
// exactly those bytes. Nothing else in the film can move, whatever the model
// writes — if it answers with a whole document, the answer is refused rather
// than applied.
//
// ── Why this is synchronous when authoring is a queued job ────────────────
// Authoring writes a whole film and does not fit in API Gateway's 29
// seconds. One element is a few hundred tokens in and a few hundred out, and
// a user watching a headline they just asked to change should not have to
// poll a document to find out whether it worked. The timeout here is set
// below the gateway's so the failure is ours and legible rather than a 504.
//
// ── One scope, not two paths ──────────────────────────────────────────────
// The plan asks that the scoped edit and the whole-film rewrite "share one
// implementation with different scope, so they cannot drift". They share
// what can honestly be shared — the contract the model writes against, the
// validator, the revision write — and differ where they must: a whole film
// is a job because it takes minutes, and one element is a request because it
// takes seconds. The thing that would actually drift is the CONTRACT, and
// there is one of those.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/internal/trendlyapis/ai"
	"github.com/idivarts/backend-sls/pkg/hfedit"
	"github.com/idivarts/backend-sls/pkg/hfproject"
	"github.com/idivarts/backend-sls/pkg/mediastore"
	"github.com/idivarts/backend-sls/pkg/mysentry"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

type aiElementRequest struct {
	// ElementID is what the user has selected. Required: with nothing
	// selected the app asks for a whole rewrite instead, which is the
	// authoring path.
	ElementID string `json:"elementId" binding:"required"`
	// Instruction is what they said, in their own words.
	Instruction string `json:"instruction" binding:"required"`
}

// aiElementTimeout is deliberately under API Gateway's 29 seconds, so a slow
// model produces our sentence rather than the gateway's 504.
const aiElementTimeout = 24 * time.Second

// maxInstruction bounds what is sent. A sentence is the shape of this
// feature; a pasted document is someone using the wrong box.
const maxInstruction = 600

// EditDesignElementWithAI rewrites one element from an instruction.
// POST /api/design/brands/:brandId/designs/:designId/element/ai
func EditDesignElementWithAI(c *gin.Context) {
	brandID, designID, ok := guard(c, true)
	if !ok {
		return
	}
	var req aiElementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	instruction := strings.TrimSpace(req.Instruction)
	if instruction == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "say what to change"})
		return
	}
	if len(instruction) > maxInstruction {
		c.JSON(http.StatusBadRequest, gin.H{"error": "that is longer than this box is for"})
		return
	}

	ctx := c.Request.Context()

	// ── The wallet, before the model ─────────────────────────────────────
	// Same two gates as authoring, kept distinct for the same reason: a
	// spent wallet wants a top-up and a locked plan wants a tier, and they
	// send the user to different places.
	orgID, _ := trendlymodels.OrgIDForBrand(brandID)
	if trendlymodels.TokensExhausted(orgID) {
		c.JSON(http.StatusPaymentRequired, gin.H{
			"error": "upgrade_required", "reason": "tokens_exhausted", "feature": "design_edit",
		})
		return
	}
	model, locked := ai.PickModelFor(ctx, brandID, openrouter.TaskDesign)
	if locked || model == "" {
		c.JSON(http.StatusPaymentRequired, gin.H{
			"error": "upgrade_required", "reason": "plan_locked", "feature": "design_edit",
		})
		return
	}

	d, err := trendlymodels.GetBrandDesign(ctx, brandID, designID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read the design"})
		return
	}
	if d == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such design"})
		return
	}
	if d.ProjectPrefix == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "This design has nothing in it yet."})
		return
	}

	raw, gerr := mediastore.Get(ctx, mediastore.TargetAttachment, d.ProjectPrefix+"/index.html")
	if gerr != nil {
		mysentry.Capture(gerr, map[string]string{"op": "design.elementAI.read", "designId": designID})
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read this film"})
		return
	}
	source := string(raw)

	el, found := hfedit.FindByID(source, req.ElementID)
	if !found {
		c.JSON(http.StatusBadRequest, gin.H{"error": "that element is no longer in this film"})
		return
	}
	if terr := editableTarget(el, source); terr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": terr.Error()})
		return
	}
	current := source[el.Start:el.End]

	genCtx, cancel := context.WithTimeout(ctx, aiElementTimeout)
	defer cancel()

	resp, cerr := openrouter.ChatCompletion(genCtx, openrouter.ChatRequest{
		Model: model,
		Messages: []openrouter.Message{
			{Role: "system", Content: elementEditSystemPrompt(sceneContext(source, el), current, req.ElementID, el.TagName)},
			{Role: "user", Content: instruction},
		},
	})
	if cerr != nil {
		reason := "The model did not answer."
		if errors.Is(cerr, context.DeadlineExceeded) {
			reason = "That took too long. Try again, or ask for something smaller."
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": reason})
		return
	}
	ai.MeterBrandUsage(brandID, resp.Usage)

	replacement, perr := parseElementReply(firstContent(resp), req.ElementID, el.TagName)
	if perr != nil {
		// ── Failure is visible, not silent ───────────────────────────────
		// The plan is explicit: "a reply that does not parse, or does not
		// pass check, leaves the previous revision in place and says so."
		// The raw answer is the only evidence of why, and it is gone the
		// moment this returns.
		mysentry.Capture(perr, map[string]string{
			"op": "design.elementAI.parse", "designId": designID,
			"head": snippet(firstContent(resp), 240),
		})
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "the model's answer could not be used",
			"message": perr.Error(),
		})
		return
	}

	edited, aerr := hfedit.Apply(source, []hfedit.Edit{{Start: el.Start, End: el.End, Text: replacement}})
	if aerr != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "the change could not be applied"})
		return
	}

	project, berr := hfproject.Build([]hfproject.File{{Path: "index.html", Content: edited}})
	if berr != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "that change would break the film",
			"message": berr.Error(),
		})
		return
	}

	rev, rerr := trendlymodels.AllocateDesignRevision(ctx, brandID, designID)
	if rerr != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not save the change"})
		return
	}
	prefix := hfproject.RevisionPrefix(brandID, designID, rev)
	if _, cperr := mediastore.CopyPrefix(ctx, mediastore.TargetAttachment, d.ProjectPrefix, prefix,
		"index.html", "player.html", "preview.html"); cperr != nil {
		mysentry.Capture(cperr, map[string]string{"op": "design.elementAI.copy", "designId": designID})
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not save the change"})
		return
	}
	if _, uerr := hfproject.Upload(ctx, brandID, designID, rev, project); uerr != nil {
		mysentry.Capture(uerr, map[string]string{"op": "design.elementAI.upload", "designId": designID})
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not save the change"})
		return
	}
	previewURL, pverr := hfproject.UploadPlayer(ctx, brandID, designID, rev, project)
	if pverr != nil {
		mysentry.Capture(pverr, map[string]string{"op": "design.elementAI.player", "designId": designID})
	}

	scenes := scenesOf(project)
	pruned, comerr := trendlymodels.CommitDesignRevision(ctx, brandID, designID, trendlymodels.DesignRevision{
		N:          rev,
		Prefix:     prefix,
		PreviewURL: previewURL,
		By:         trendlymodels.DesignRevisionAIEdit,
		Note:       revisionNote(instruction),
		DurationMs: project.DurationMs,
		Scenes:     scenes,
		Clips:      clipsOf(project),
	})
	if comerr != nil {
		mysentry.Capture(comerr, map[string]string{"op": "design.elementAI.commit", "designId": designID})
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not save the change"})
		return
	}
	dropRevisionFiles(ctx, pruned)

	c.JSON(http.StatusOK, gin.H{
		"revision":   rev,
		"previewUrl": previewURL,
		"durationMs": project.DurationMs,
	})
}

// sceneContext is the scene the element sits in, for the model to read.
//
// Context and not just the element, because "make this headline bigger"
// needs to know what else is on the frame to stay in proportion, and
// "match the other text" is meaningless without it. Bounded, because the
// point of a scoped edit is that it is small: a scene larger than this is
// sent as the element alone rather than as half a scene cut mid-tag.
func sceneContext(source string, el hfedit.Element) string {
	const maxScene = 6000
	for _, s := range findScenesAround(source, el) {
		if len(s) <= maxScene {
			return s
		}
	}
	return ""
}

// findScenesAround returns the enclosing [data-scene] element's markup, if
// the element has one.
//
// The containment test is a FILTER and not part of the predicate, because
// FindAll runs its predicate against the start tag — where End is not known
// yet. Written the other way round it silently matched nothing, which
// looked exactly like "this element has no scene".
func findScenesAround(source string, el hfedit.Element) []string {
	var out []string
	for _, s := range hfedit.FindAll(source, func(e hfedit.Element) bool {
		_, has := e.Attrs["data-scene"]
		return has
	}) {
		if s.Start <= el.Start && s.End >= el.End {
			out = append(out, source[s.Start:s.End])
		}
	}
	return out
}

// elementEditSystemPrompt tells the model exactly what it may return.
//
// The rules are stated as refusals rather than preferences because the
// parser enforces them: an answer that breaks one is thrown away, and a
// model told clearly is a model that wastes fewer of the org's tokens.
func elementEditSystemPrompt(scene, current, elementID, tagName string) string {
	var b strings.Builder
	b.WriteString(`You are editing ONE element of a short social video, written as an HTML document animated with GSAP.

You will be given the element and, for context, the scene it sits in. Change ONLY the element, and answer with ONLY its replacement markup.

RULES — an answer that breaks any of these is thrown away and the user is told it failed:
1. Answer with the element and nothing else. No explanation, no markdown fence, no <html> or <body>, no second element.
2. Keep id="` + elementID + `". It is how the film is addressed; an answer that renames or drops it cannot be applied.
3. Keep the <` + tagName + `> tag unless the instruction plainly asks for a different kind of element.
4. Keep class="clip", data-start and data-duration unless the instruction is about timing. The timeline is built from them.
5. Style inline, with the brand's own tokens (var(--brand-…)) where the scene uses them. Do not add a <style> block, a <script>, or an id that is not this element's.
6. Keep text inside the middle 80% of the frame and never tween letterSpacing, width, height, margin or padding — the renderer rejects those.

`)
	if scene != "" {
		b.WriteString("THE SCENE THIS SITS IN, for context only — do not return it:\n")
		b.WriteString(scene)
		b.WriteString("\n\n")
	}
	b.WriteString("THE ELEMENT TO REPLACE:\n")
	b.WriteString(current)
	return b.String()
}

// parseElementReply takes the model's answer back to one element.
//
// Everything here is a refusal the plan asks for by name. A reply that is a
// whole document, or two elements, or an element under a different id, is
// not an edit of this element — applying it anyway is exactly the silent
// change of a different scene that scoping exists to prevent.
func parseElementReply(reply, elementID, tagName string) (string, error) {
	out := strings.TrimSpace(reply)
	if out == "" {
		return "", fmt.Errorf("the model answered with nothing")
	}
	// A fenced block is common enough from every model to be worth
	// unwrapping rather than refusing.
	if strings.HasPrefix(out, "```") {
		if i := strings.Index(out, "\n"); i >= 0 {
			out = out[i+1:]
		}
		if i := strings.LastIndex(out, "```"); i >= 0 {
			out = out[:i]
		}
		out = strings.TrimSpace(out)
	}
	if out == "" {
		return "", fmt.Errorf("the model answered with an empty block")
	}
	lower := strings.ToLower(out)
	if strings.Contains(lower, "<!doctype") || strings.Contains(lower, "<body") {
		return "", fmt.Errorf("the model rewrote the whole film instead of the element")
	}
	// ── No script, and the prompt says so ────────────────────────────────
	// A film already contains model-written JavaScript — its GSAP timeline
	// — so this is not a new class of risk. But it is a rule the prompt
	// states, the element being replaced is not where a timeline belongs,
	// and a page served from the distribution every brand's projects share
	// is not somewhere to let an unasked-for script in because checking was
	// inconvenient.
	if strings.Contains(lower, "<script") || strings.Contains(lower, "javascript:") {
		return "", fmt.Errorf("the answer contains a script, which an element may not")
	}
	if onEventAttr.MatchString(out) {
		return "", fmt.Errorf("the answer contains an event handler, which an element may not")
	}

	found := hfedit.FindAll(out, func(e hfedit.Element) bool { return e.Attrs["id"] == elementID })
	if len(found) == 0 {
		return "", fmt.Errorf("the answer does not carry id=%q, so it cannot be put back", elementID)
	}
	if len(found) > 1 {
		return "", fmt.Errorf("the answer repeats id=%q", elementID)
	}
	el := found[0]
	// The element must BE the answer, not sit inside a wrapper the model
	// invented — a wrapper would land in the document as a new element
	// nobody asked for.
	if strings.TrimSpace(out[:el.Start]) != "" || strings.TrimSpace(out[el.End:]) != "" {
		return "", fmt.Errorf("the answer has more in it than the element")
	}
	// A changed tag is allowed: "make this a paragraph" is a legitimate
	// instruction, and refusing it would make the panel feel arbitrary.
	_ = tagName
	return out, nil
}

// onEventAttr matches an inline event handler — onclick, onload, onerror
// and the rest — written as an attribute. Deliberately broad: the rule is
// "no handlers", not "no handlers I thought of".
var onEventAttr = regexp.MustCompile(`(?i)\son[a-z]+\s*=`)
