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
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/internal/studiojobs"
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
// GenerateScene asks for ONE scene's HTML.
// POST …/studios/:studioId/scenes/:sceneId/generate
//
// ⚠️ THIS NO LONGER GENERATES ANYTHING. It validates, charges the gate, marks
// the scene queued and hands the work to the studio queue — 202, not 200.
//
// It used to do the model call inline, and could not. A design is a frontier
// model emitting a whole HTML document: 25–40 seconds against a Lambda capped
// at 30. On dev, three of five scenes in a real run died at exactly 29.999s,
// and because the request context carries the Lambda deadline, the write that
// would have recorded the failure used an expired context and was dropped. The
// scenes could not even say they had failed. One wrote its layers and died
// before the update clearing `pending`, so it held a finished design and still
// said "Making…".
//
// There is no version of that which works. 30 seconds is the ceiling on every
// HTTP function here, the gateway gives up at 29 regardless of the Lambda, and
// the work genuinely takes longer. So the request stops waiting, and the
// client watches the scene document instead — which it was already subscribed
// to, so the rail fills by itself as each design lands.
func GenerateScene(c *gin.Context) {
	brandID, contentID, studioID, ok := guard(c, true)
	if !ok {
		return
	}
	var req generateSceneRequest
	_ = c.ShouldBindJSON(&req)

	ctx := c.Request.Context()
	sceneID := c.Param("sceneId")

	// Refused up front rather than after the scene is marked queued. Without
	// this a missing env var would mark the scene pending and enqueue into
	// nothing — a scene stuck at "Making…", which is the precise failure this
	// whole change exists to delete.
	if !studiojobs.Configured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "generator_unavailable",
			"message": "Scene generation is not available right now. Please try again later.",
		})
		return
	}

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

	// The gate stays HERE, in the request, even though the spending happens in
	// the worker. A 402 is something the user has to see and act on, and a
	// refusal discovered by a background job is a refusal nobody reads.
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

	// Everything the worker needs goes ON THE SCENE, so the job message stays
	// three ids and a type. A message carrying the brief would be a second
	// copy of it, and the two would disagree the first time someone edited the
	// scene between the enqueue and the run.
	// Built FROM the scene's existing record, not over it. A scene may carry
	// cast ids, a chain or a start frame that belong to it rather than to this
	// request, and writing a fresh struct would silently drop them — the kind
	// of loss that only shows up later as a face changing between clips.
	gen := trendlymodels.SceneGeneration{}
	if sc.Gen != nil {
		gen = *sc.Gen
	}
	gen.Model = model
	gen.Prompt = brief
	gen.Status = trendlymodels.SceneGenQueued
	gen.QueuedAt = nowMs()
	gen.Error = ""
	if gen.Params == nil {
		gen.Params = map[string]interface{}{}
	}
	gen.Params["approach"] = approach

	if err := trendlymodels.MarkSceneGenerationQueued(ctx, brandID, contentID, studioID, sceneID,
		&gen); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not queue the scene"})
		return
	}

	if err := studiojobs.Enqueue(studiojobs.Message{
		Type:      studiojobs.JobSceneDesign,
		BrandID:   brandID,
		ContentID: contentID,
		StudioID:  studioID,
		SceneID:   sceneID,
	}); err != nil {
		// The scene is marked queued and nothing is coming, so say so on the
		// scene as well as in the response. A pending scene with no job is the
		// one state this design must never leave behind.
		_ = trendlymodels.MarkSceneGenerationFailed(ctx, brandID, contentID, studioID, sceneID,
			"we couldn't start the generation")
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "enqueue_failed",
			"message": "We couldn't start that scene. Please try again.",
		})
		return
	}

	// 202, and the scene as it now reads. The client watches Firestore for the
	// design; it does not poll this.
	reloadAndRespondWith(c, http.StatusAccepted, brandID, contentID, studioID)
}

// ExecuteSceneDesignJob is the worker half: the model call and the writes.
//
// Runs on studio_sqs, which has 900 seconds. The budget helpers still apply —
// the numbers are just no longer absurd — because a worker that spends its
// entire ceiling on the model has nothing left to record what happened, and
// that is the failure mode this moved to escape.
//
// ⚠️ ALWAYS RECORDS AN OUTCOME. The caller swallows the error to stop SQS
// replaying the batch, and a replayed design is a second frontier-model call
// for a scene the user already paid for. So every path here ends with the
// scene either done or failed, never still queued.
func ExecuteSceneDesignJob(ctx context.Context, msg studiojobs.Message) error {
	brandID, contentID, studioID, sceneID := msg.BrandID, msg.ContentID, msg.StudioID, msg.SceneID

	sc, err := trendlymodels.GetStudioScene(ctx, brandID, contentID, studioID, sceneID)
	if err != nil {
		return err
	}
	if sc == nil {
		// Deleted between the enqueue and the run. An ordinary outcome.
		log.Printf("studiojobs: scene_design for a missing scene %s/%s/%s/%s",
			brandID, contentID, studioID, sceneID)
		return nil
	}
	if sc.Gen == nil || sc.Gen.Prompt == "" {
		return trendlymodels.MarkSceneGenerationFailed(ctx, brandID, contentID, studioID, sceneID,
			"there was nothing to make this scene from")
	}
	// Idempotency. SQS is at-least-once, and a redelivery of a job that
	// already finished would charge for the same design twice and overwrite a
	// scene the user may have edited since.
	if sc.Gen.Status != trendlymodels.SceneGenQueued {
		log.Printf("studiojobs: scene_design already %s for %s, skipping", sc.Gen.Status, sceneID)
		return nil
	}

	s, err := trendlymodels.GetStudio(ctx, brandID, contentID, studioID)
	if err != nil || s == nil {
		return trendlymodels.MarkSceneGenerationFailed(ctx, brandID, contentID, studioID, sceneID,
			"the studio was deleted")
	}

	approach := trendlymodels.SceneMotion
	if a, _ := sc.Gen.Params["approach"].(string); a == trendlymodels.SceneComposite {
		approach = trendlymodels.SceneComposite
	}
	brief := sc.Gen.Prompt

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

	genCtx, cancelGen := modelBudget(ctx)
	defer cancelGen()

	resp, err := openrouter.ChatCompletion(genCtx, openrouter.ChatRequest{
		Model: sc.Gen.Model,
		Messages: []openrouter.Message{
			{Role: "system", Content: sys},
			{Role: "user", Content: user},
		},
	})
	bookCtx, cancelBook := bookkeepingCtx(ctx)
	defer cancelBook()

	if err != nil {
		reason := "the model did not answer"
		if errors.Is(err, context.DeadlineExceeded) {
			reason = "the design took too long. Try again, or shorten the brief."
		}
		return trendlymodels.MarkSceneGenerationFailed(bookCtx, brandID, contentID, studioID, sceneID, reason)
	}
	ai.MeterBrandUsage(brandID, resp.Usage)

	// Model output, straight into a Firestore string field — the same hazard
	// that threw away a whole director plan. See safeForFirestore.
	html := safeForFirestore(cleanHTML(firstContent(resp)))
	if html == "" {
		return trendlymodels.MarkSceneGenerationFailed(bookCtx, brandID, contentID, studioID, sceneID,
			"the model returned no document")
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
	if err := trendlymodels.SetSceneLayers(bookCtx, brandID, contentID, studioID, sceneID, layers, approach); err != nil {
		// The design exists but is not saved. Said on the scene, because a
		// scene left queued is the one thing this must never produce.
		_ = trendlymodels.MarkSceneGenerationFailed(bookCtx, brandID, contentID, studioID, sceneID,
			"we couldn't save that scene")
		return err
	}
	if err := trendlymodels.UpdateStudioScene(bookCtx, brandID, contentID, studioID, sceneID, []firestore.Update{
		{Path: "pending", Value: false},
		{Path: "gen.status", Value: trendlymodels.SceneGenDone},
		{Path: "gen.error", Value: ""},
	}); err != nil {
		return err
	}

	// This may have been the last scene the run was waiting on.
	_ = trendlymodels.SettleStudioRunIfComplete(bookCtx, brandID, contentID, studioID, sc.RunID)
	return nil
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
	// Deliberately the model's marker rather than a second copy of the same
	// three updates: it also settles the run, and a scene that failed here
	// would otherwise hold its run open forever.
	return trendlymodels.MarkSceneGenerationFailed(ctx, brandID, contentID, studioID, sceneID, reason)
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

// ── Spending a Lambda's 30 seconds ─────────────────────────────────────────
//
// Every HTTP-facing function in this service is capped at 30 seconds, and the
// request context carries that deadline. A model call is allowed to use most
// of it; what it is NOT allowed to do is use all of it, because the writes
// that record the outcome run on the same context and a context past its
// deadline fails every one of them — silently, since their errors are ignored
// on purpose so a saved scene is never reported as lost.
//
// The result of getting this wrong is the thing this file now works hard to
// avoid: a scene that says "Making…" with nothing coming.

// bookkeepingReserve is the MINIMUM kept back for writing down what happened —
// what a 30-second HTTP request can spare.
//
// ⚠️ IT IS A FLOOR, NOT THE WINDOW. Treating it as the window is a mistake I
// made and shipped: the generation moved to a worker with 900 seconds, and the
// writes that follow it were still being squeezed into five. Two of three
// scenes then saved their design and failed to record that they had —
//
//	scene design failed … rpc error: code = DeadlineExceeded
//	Duration: 43505.75 ms
//
// — leaving exactly the half-written scene the move to a queue was meant to
// abolish: a finished design still saying "Making…".
const bookkeepingReserve = 6 * time.Second

// bookkeepingMax caps the window where there is time to spare.
//
// SetSceneLayers writes a whole HTML document and diffs the scene's media
// references; UpdateStudioScene follows it. Neither is slow, but five seconds
// for both was not enough in practice, and a worker holding 900 seconds has no
// reason to be stingy. Capped all the same, because a write that cannot finish
// in a minute is not going to.
const bookkeepingMax = 60 * time.Second

// modelBudget gives the generation everything except the reserve.
//
// Falls back to a fixed budget when the context carries no deadline, which is
// the case in tests and in any local run outside Lambda.
func modelBudget(ctx context.Context) (context.Context, context.CancelFunc) {
	deadline, ok := ctx.Deadline()
	if !ok {
		return context.WithTimeout(ctx, 30*time.Second-bookkeepingReserve)
	}
	budget := time.Until(deadline) - bookkeepingReserve
	if budget <= 0 {
		// Already inside the reserve. Let the caller fail immediately and
		// spend what is left saying so, rather than starting a call that
		// cannot finish and costs money either way.
		budget = time.Millisecond
	}
	return context.WithTimeout(ctx, budget)
}

// bookkeepingCtx is for the writes that record an outcome.
//
// Detached from the caller, so the generation's own expiry — or a client that
// has already given up — cannot cancel the write that says what happened to
// the scene. Bounded all the same, because an uncancellable write would simply
// hang until the Lambda is killed.
//
// The window SCALES with what the caller actually has. An HTTP handler can
// spare seconds; the queue worker has minutes. Giving the worker the handler's
// reserve is what stranded two scenes with their designs saved and their
// status unwritten.
func bookkeepingCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	window := bookkeepingReserve - time.Second

	deadline, ok := ctx.Deadline()
	if !ok {
		// No deadline at all: outside Lambda, or a detached context. Take the
		// cap rather than the floor — there is nothing to be frugal for.
		return context.WithTimeout(context.WithoutCancel(ctx), bookkeepingMax)
	}
	// A second back from whatever is left, so the writes end before the
	// process does and have a chance to report their own failure.
	if spare := time.Until(deadline) - time.Second; spare > window {
		window = min(spare, bookkeepingMax)
	}
	return context.WithTimeout(context.WithoutCancel(ctx), window)
}
