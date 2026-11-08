package studio

// Motion-graphics generation, one scene document at a time.
//
// ── Why this is two endpoints and not one ───────────────────────────────────
// The obvious shape — "generate the whole thing" — cannot work here, and not
// only because a Lambda has thirty seconds. Six sequential HTML generations is
// minutes of work, and a user watching a spinner for minutes has no idea
// whether it is working, how much it will cost, or how to stop it.
//
// So it splits where the user's attention does:
//
//	POST …/plan-scenes          ONE cheap call. Returns a scene list and writes
//	                            every scene immediately — empty and marked
//	                            pending. The rail fills in about a second, so
//	                            the shape of the thing is on screen before any
//	                            of the expensive work starts.
//	POST …/scenes/:id/generate  ONE scene's HTML. The client walks the list.
//
// That ordering is the whole user-visible benefit: you see six scenes appear,
// then watch them fill one by one, and you can stop after two. It also keeps
// every request comfortably inside the Lambda budget, and it means a failure
// costs ONE scene rather than the run.
//
// ⚠️ Both endpoints are gated on BOTH SIDES. They refuse with a `reason` the UI
// reacts to — `tokens_exhausted` for a spent wallet, `plan_locked` when the
// brand's plan unlocks no model for design work — because a gate that only
// exists in the UI is not a gate, and one that only exists here arrives after
// the user has already committed.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/internal/trendlyapis/ai"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// How many scenes one plan may produce.
//
// Eight is a judgement about watchability rather than a technical limit: a
// 30-second reel with more than eight cuts is a montage, and the planner will
// happily produce fifteen three-second scenes if nothing stops it.
const maxPlannedScenes = 8

type planScenesRequest struct {
	// Brief is what the user asked for, in their words.
	Brief string `json:"brief" binding:"required"`
	// SceneCount is a request, not an instruction: the planner may return
	// fewer if the brief does not carry that many beats. Omitted lets it
	// choose.
	SceneCount int `json:"sceneCount"`
}

// plannedScene is one beat the planner proposes.
type plannedScene struct {
	Title      string `json:"title"`
	Brief      string `json:"brief"`
	DurationMs int    `json:"durationMs"`
	Narration  string `json:"narration"`
	// Transition is how this scene arrives. The planner is told to leave the
	// first one alone and to use the document's preset elsewhere.
	Transition string `json:"transition"`
}

// PlanScenes lays out the scene list and writes every scene empty.
// POST …/studios/:studioId/plan-scenes
func PlanScenes(c *gin.Context) {
	brandID, contentID, studioID, ok := guard(c, true)
	if !ok {
		return
	}
	var req planScenesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	s, err := trendlymodels.GetStudio(ctx, brandID, contentID, studioID)
	if err != nil || s == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such studio"})
		return
	}

	model, blocked := gateDesign(c, brandID)
	if blocked {
		return
	}

	want := req.SceneCount
	if want <= 0 {
		want = 4
	}
	if want > maxPlannedScenes {
		want = maxPlannedScenes
	}

	content, _ := trendlymodels.GetContent(brandID, contentID)
	sys := "You plan the SCENES of a short social video or carousel.\n\n" +
		"Return STRICTLY a JSON array, no markdown and no commentary. Each element:\n" +
		`  {"title": "...", "brief": "...", "durationMs": 3000, "narration": "...", "transition": "cut"}` + "\n\n" +
		"RULES\n" +
		"- One BEAT per scene. A beat is one idea the viewer takes away; two ideas in a scene is two scenes.\n" +
		"- `brief` is an instruction to whoever designs that single frame. Say what is ON it, not how it feels.\n" +
		"- `narration` is what is SAID over the scene, or \"\" for none. Speech runs at about 14 characters a " +
		"second, so a 3000ms scene holds roughly 40 characters. A line that cannot be said inside its scene is refused.\n" +
		"- `narration` MUST come from the material you are given — the script, the caption, or the brand's own " +
		"documented facts. Do NOT invent a customer, a statistic, a price or a named third party.\n" +
		"- `transition` is \"cut\" for the first scene always, and \"cut\" elsewhere unless a join genuinely helps.\n" +
		"- Open on the hook. The first scene earns the second."

	user := fmt.Sprintf("Format: %s (%dx%d).\nScenes wanted: %d.\n\nBrief: %s",
		s.Format, s.Width, s.Height, want, strings.TrimSpace(req.Brief))
	if content != nil {
		if content.Script != "" {
			user += "\n\nThe script to work from:\n" + content.Script
		}
		if content.Caption != "" {
			user += "\n\nThe caption:\n" + content.Caption
		}
		if content.Title != "" {
			user += "\n\nThe post is called: " + content.Title
		}
	}

	resp, err := openrouter.ChatCompletion(ctx, openrouter.ChatRequest{
		Model: model,
		Messages: []openrouter.Message{
			{Role: "system", Content: sys + ai.DesignContextFor(brandID)},
			{Role: "user", Content: user},
		},
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "planning_failed",
			"message": "We couldn't plan those scenes. Please try again.",
		})
		return
	}
	ai.MeterBrandUsage(brandID, resp.Usage)

	planned := parsePlannedScenes(firstContent(resp))
	if len(planned) == 0 {
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "planning_empty",
			"message": "The planner didn't return any scenes. Try describing it differently.",
		})
		return
	}
	if len(planned) > maxPlannedScenes {
		planned = planned[:maxPlannedScenes]
	}

	// A run document for this batch, BEFORE the scenes, so every scene can
	// carry its id.
	//
	// plan-scenes used to create no run at all, which left the rail unable to
	// tell "these six are one thing the AI planned" from six unrelated scenes,
	// and left the user with no record of a batch that spent real money. A
	// director plan already got a run for exactly those reasons; a planned
	// batch is the same thing with a shorter approval step, so it gets one too.
	//
	// A failure here is NOT fatal: the scenes are the valuable part, and
	// refusing to plan because the bookkeeping row could not be written would
	// be the tool getting in the way. The scenes are then ungrouped, which is
	// how the rail already draws hand-made ones.
	runID, runErr := trendlymodels.CreateStudioRun(ctx, brandID, contentID, studioID, &trendlymodels.StudioRun{
		Status: trendlymodels.RunRunning,
		Brief:  strings.TrimSpace(req.Brief),
		Model:  model,
		// Approved by the act of asking: plan-scenes is the one-tap path, with
		// no storyboard to read first. Recorded so a run is never in a state
		// nobody chose.
		ApprovedAt: time.Now().UnixMilli(),
		CreatedBy:  c.GetString("userId"),
	})
	if runErr != nil {
		runID = ""
	}

	// Write them ALL before returning, empty and pending. This is what puts
	// the shape of the video on the rail in about a second — the expensive
	// per-scene work then fills in visibly rather than behind a spinner.
	created := make([]string, 0, len(planned))
	for _, p := range planned {
		duration := p.DurationMs
		if duration <= 0 {
			duration = defaultSceneMs(s.Format)
		}
		sc := &trendlymodels.StudioScene{
			Title:       strings.TrimSpace(p.Title),
			DurationMs:  duration,
			PrimaryKind: trendlymodels.SceneEmpty,
			Pending:     true,
			RunID:       runID,
			Layers:      []trendlymodels.StudioLayer{},
			// The brief travels on the scene so the per-scene generate call is
			// self-contained: the client sends a scene id, not a transcript of
			// what the planner decided.
			Gen: &trendlymodels.SceneGeneration{
				Model:  model,
				Prompt: strings.TrimSpace(p.Brief),
				Status: "queued",
			},
		}
		if line := strings.TrimSpace(p.Narration); line != "" {
			// Provenance: the planner was told to take narration from the
			// supplied material, so it is recorded as a paraphrase of it
			// rather than as a quotation we cannot verify word for word.
			sc.Narration = &trendlymodels.SceneNarration{
				Line:           line,
				Source:         trendlymodels.NarrationParaphrase,
				FitToNarration: true,
			}
		}
		if t := strings.TrimSpace(p.Transition); t != "" && knownTransition(t) && t != trendlymodels.TransitionCut {
			sc.TransitionIn = &trendlymodels.Transition{Kind: t, DurationMs: 400}
		}
		id, err := trendlymodels.CreateStudioScene(ctx, brandID, contentID, studioID, sc)
		if err != nil {
			// Stop at the first failure rather than pressing on: a partial
			// plan is still usable, and the user can see exactly how far it
			// got instead of being told it worked.
			break
		}
		created = append(created, id)
	}

	// Which scenes were this run's, in plan order — so a run can be undone
	// later without guessing, exactly as an approved director run records.
	if runID != "" && len(created) > 0 {
		_ = trendlymodels.UpdateStudioRun(ctx, brandID, contentID, studioID, runID, []firestore.Update{
			{Path: "sceneIds", Value: created},
		})
	}

	reloadAndRespond(c, brandID, contentID, studioID)
}

type generateSceneRequest struct {
	// Approach: motion (an animated document) or composite (a still, composed
	// from elements and images). Defaults to motion for a timed format.
	Approach string `json:"approach"`
	// Brief overrides whatever the planner put on the scene.
	Brief string `json:"brief"`
}

// GenerateScene writes ONE scene's HTML.
// POST …/studios/:studioId/scenes/:sceneId/generate
//
// One scene per request, which is what makes the rail fill visibly and a
// failure cost one scene rather than the whole run.
func GenerateScene(c *gin.Context) {
	brandID, contentID, studioID, ok := guard(c, true)
	if !ok {
		return
	}
	var req generateSceneRequest
	_ = c.ShouldBindJSON(&req)

	ctx := c.Request.Context()
	sceneID := c.Param("sceneId")

	s, err := trendlymodels.GetStudio(ctx, brandID, contentID, studioID)
	if err != nil || s == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such studio"})
		return
	}
	sc, err := trendlymodels.GetStudioScene(ctx, brandID, contentID, studioID, sceneID)
	if err != nil || sc == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such scene"})
		return
	}

	model, blocked := gateDesign(c, brandID)
	if blocked {
		return
	}

	brief := strings.TrimSpace(req.Brief)
	if brief == "" && sc.Gen != nil {
		brief = sc.Gen.Prompt
	}
	if brief == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "no_brief",
			"message": "Say what should be on this scene.",
		})
		return
	}

	approach := req.Approach
	if approach != trendlymodels.SceneComposite {
		approach = trendlymodels.SceneMotion
	}

	sys := sceneSystemPrompt(approach, s.Width, s.Height, sc.DurationMs) +
		ai.DesignContextFor(brandID) +
		"\n\nBRAND CSS (declare these once at the top of the document and use them):\n" +
		ai.BrandKitCSS(brandID, "")

	user := "Design this ONE scene.\n\nWhat is on it: " + brief
	if sc.Narration != nil && sc.Narration.Line != "" {
		// The spoken line matters to the LAYOUT, not just the sound: type that
		// repeats the voiceover word for word is the commonest way a generated
		// scene reads as cheap.
		user += "\n\nThis is said over it (do NOT put the whole line on screen as text): " +
			sc.Narration.Line
	}

	resp, err := openrouter.ChatCompletion(ctx, openrouter.ChatRequest{
		Model: model,
		Messages: []openrouter.Message{
			{Role: "system", Content: sys},
			{Role: "user", Content: user},
		},
	})
	if err != nil {
		_ = markSceneGenFailed(ctx, brandID, contentID, studioID, sceneID, "the model did not answer")
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "generation_failed",
			"message": "We couldn't design that scene. Please try again.",
		})
		return
	}
	ai.MeterBrandUsage(brandID, resp.Usage)

	html := cleanHTML(firstContent(resp))
	if html == "" {
		_ = markSceneGenFailed(ctx, brandID, contentID, studioID, sceneID, "the model returned no document")
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "generation_empty",
			"message": "That came back empty. Please try again.",
		})
		return
	}

	// The HTML layer REPLACES whatever was there. A generated scene is a take,
	// not an addition — appending would leave the previous attempt underneath,
	// visible wherever the new one does not cover it.
	layers := []trendlymodels.StudioLayer{{
		ID:   fmt.Sprintf("l_gen_%d", nowMs()),
		Kind: "html",
		Slot: trendlymodels.SlotFull,
		HTML: html,
	}}
	if err := trendlymodels.SetSceneLayers(ctx, brandID, contentID, studioID, sceneID, layers, approach); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not save the scene"})
		return
	}
	if err := trendlymodels.UpdateStudioScene(ctx, brandID, contentID, studioID, sceneID, []firestore.Update{
		{Path: "pending", Value: false},
		{Path: "gen", Value: &trendlymodels.SceneGeneration{
			Model:  model,
			Prompt: brief,
			Status: "done",
		}},
	}); err != nil {
		// The scene HAS its design; only the bookkeeping failed. Reporting an
		// error here would make the user regenerate something that worked.
		c.Header("X-Scene-Bookkeeping", "failed")
	}

	reloadAndRespond(c, brandID, contentID, studioID)
}

// gateDesign refuses the call when the wallet is empty or the plan unlocks no
// design model, and returns the model to use otherwise.
//
// Both refusals carry a `reason`, because the UI reacts differently to each: a
// spent wallet wants a top-up, a locked plan wants a tier. Answering both as
// "upgrade" sends half the users to the wrong screen.
func gateDesign(c *gin.Context, brandID string) (model string, blocked bool) {
	orgID, _ := trendlymodels.OrgIDForBrand(brandID)
	if trendlymodels.TokensExhausted(orgID) {
		c.JSON(http.StatusPaymentRequired, gin.H{
			"error":   "upgrade_required",
			"reason":  "tokens_exhausted",
			"feature": "studio_generate",
		})
		return "", true
	}
	model, locked := ai.PickModelFor(c.Request.Context(), brandID, openrouter.TaskDesign)
	if locked || model == "" {
		c.JSON(http.StatusPaymentRequired, gin.H{
			"error":   "upgrade_required",
			"reason":  "plan_locked",
			"feature": "studio_generate",
		})
		return "", true
	}
	return model, false
}

func markSceneGenFailed(ctx context.Context, brandID, contentID, studioID, sceneID, reason string) error {
	return trendlymodels.UpdateStudioScene(ctx, brandID, contentID, studioID, sceneID, []firestore.Update{
		{Path: "pending", Value: false},
		{Path: "gen.status", Value: "failed"},
		{Path: "gen.error", Value: reason},
	})
}

// sceneSystemPrompt is the contract for ONE scene's document.
//
// The important difference from the old whole-design prompt: there is no
// carousel wrapper and no sibling slides. A scene IS the frame, which is why it
// can be regenerated on its own — the thing that was impossible when every
// scene was a child of one document.
func sceneSystemPrompt(approach string, width, height, durationMs int) string {
	base := fmt.Sprintf(
		"You write ONE self-contained HTML document that fills a single %dx%d frame.\n\n"+
			"Return ONLY the HTML. No markdown fence, no commentary, no <html> or <body> wrapper.\n\n"+
			"HARD RULES\n"+
			"- Inline CSS only. No external files, no scripts, no web fonts beyond the brand's.\n"+
			"- The root element is ONE <section style=\"width:%dpx;height:%dpx;position:relative;overflow:hidden\">.\n"+
			"- There are NO sibling scenes in this document. This file is one scene; the timeline holds the rest.\n"+
			"- Keep every piece of text well inside the frame. Size type to fit — overflow is the one failure that cannot be fixed later.\n"+
			"- Give every editable text element a data-el=\"<unique-id>\", so a person can click it and retype it.\n",
		width, height, width, height)

	if approach == trendlymodels.SceneComposite {
		return base +
			"\nTHIS SCENE IS A STILL. It is captured once, so it must read at a glance and must not\n" +
			"depend on movement. Compose it: place images and elements deliberately, use the brand's\n" +
			"palette, and let one thing dominate. No CSS animations — anything that moves is wasted\n" +
			"work here and will be captured mid-flight."
	}

	return base + fmt.Sprintf(
		"\nTHIS SCENE ANIMATES over %dms.\n"+
			"- Use CSS animations and transitions on the elements inside the section.\n"+
			"- Every animation must be FINISHED by %dms. A scene still moving when it cuts reads as a glitch.\n"+
			"- Animations must be deterministic and seekable: no JavaScript, no randomness, no infinite loops.\n"+
			"  The renderer seeks to a frame time and captures; anything that depends on wall-clock\n"+
			"  time or a loop count renders differently every run.\n"+
			"- Entrances are enough. Two elements arriving well beats six arriving at once.",
		durationMs, durationMs)
}

func firstContent(resp *openrouter.ChatResponse) string {
	if resp == nil || len(resp.Choices) == 0 {
		return ""
	}
	return resp.Choices[0].Message.Content
}

// cleanHTML strips the markdown fence models add despite being told not to.
//
// Not a nicety: a document that starts with ```html renders those three
// characters as text in the top-left corner of the scene, and it is the kind of
// defect that survives review because it looks like a design choice.
func cleanHTML(raw string) string {
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(s, "```") {
		if i := strings.Index(s, "\n"); i >= 0 {
			s = s[i+1:]
		}
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	}
	s = strings.TrimSpace(s)
	if !strings.Contains(strings.ToLower(s), "<section") {
		// Anything without the root element the prompt demands is not a scene.
		// Better to fail than to store a fragment that renders as a blank frame.
		return ""
	}
	return s
}

// parsePlannedScenes reads the planner's JSON array, tolerating a fence.
func parsePlannedScenes(raw string) []plannedScene {
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(s, "```") {
		if i := strings.Index(s, "\n"); i >= 0 {
			s = s[i+1:]
		}
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	}
	// Models occasionally wrap the array in an object. Finding the first
	// bracket is cheaper and more reliable than a second model call to fix it.
	if i := strings.Index(s, "["); i > 0 {
		if j := strings.LastIndex(s, "]"); j > i {
			s = s[i : j+1]
		}
	}
	var out []plannedScene
	if err := json.Unmarshal([]byte(strings.TrimSpace(s)), &out); err != nil {
		return nil
	}
	return out
}
