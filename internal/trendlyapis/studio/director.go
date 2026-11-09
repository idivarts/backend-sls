package studio

// The AI director: plan, read, approve, materialise.
//
// ── The shape, and why it is four steps ─────────────────────────────────────
//	POST …/plan              one model call → a stored plan + a storyboard
//	GET  …/runs              what has been planned before
//	POST …/runs/:id/approve  the plan becomes scenes
//	then                     the per-scene endpoints fill them
//
// The plan is STORED rather than returned and forgotten, because a person reads
// a storyboard, thinks about it, and approves it — possibly on another device.
// A plan that lived only in a response would have to be re-planned, which costs
// money and produces a different plan.
//
// ⚠️ WHAT APPROVAL DOES NOT DO: it does not run a graph. Approval materialises
// the plan into scenes — all of them, immediately, marked pending — and the
// per-scene generate endpoints fill them one at a time, driven by the client.
// That is the same choice as Phase 3's motion generation and for the same three
// reasons: each request stays inside the Lambda budget, the rail fills visibly
// instead of behind a spinner, and the user can stop after two scenes without
// paying for six. The plan's `dependsOn` order is preserved in `sceneOrder`, so
// a chained beat is still filled after the beat it continues.
//
// The parallel run graph with a concurrency cap and per-node fallbacks that the
// plan document describes is NOT built. What exists is the sequential walk.
//
// ⚠️ A PLAN WITH A FATAL PROBLEM CANNOT BE APPROVED, at the API, regardless of
// what any client sends. The validator is the gate — warnings are shown and can
// be overridden, fatals never are.

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"cloud.google.com/go/firestore"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/internal/studiojobs"
	"github.com/idivarts/backend-sls/internal/trendlyapis/ai"
	"github.com/idivarts/backend-sls/internal/trendlyapis/ai/director"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

type planRunRequest struct {
	Brief string `json:"brief" binding:"required"`
	// Mode is the user's intent: economy | balanced | cinematic. They pick an
	// intent, never a model.
	Mode string `json:"mode"`
	// TargetSeconds is what they asked for. 0 lets the planner decide, which
	// it then has to say out loud in its summary.
	TargetSeconds int `json:"targetSeconds"`
}

// PlanRun accepts a planning request and hands it to the studio queue.
// POST …/studios/:studioId/plan  →  202 {runId, status:"planning"}
//
// ⚠️ IT DOES NOT WAIT FOR THE PLAN, and it cannot. API Gateway's integration
// timeout is 29 seconds and is not configurable; a director plan is one or two
// calls to a frontier reasoning model, each emitting a whole storyboard as
// JSON. Measured against the deployed dev stack this returned 504 at 30s on
// both a cold and a warm call, with NO run stored, because the write happened
// after the planner answered. Raising the Lambda timeout cannot help — the
// gateway gives up first.
//
// So the row is created in `planning` state and the work moves to studio_sqs.
// The client polls GET …/runs/:runId, which already existed, because a plan was
// always meant to be read back later rather than only returned once.
//
// What stays SYNCHRONOUS is everything the user must be told immediately:
// whether the studio and content exist, whether their plan or wallet allows a
// plan at all (402 with a reason, so the UI shows the right prompt in context),
// and whether any models are switched on. Those are cheap, and deferring them
// would turn an instant, explainable refusal into a spinner that resolves into
// one.
func PlanRun(c *gin.Context) {
	brandID, contentID, studioID, ok := guard(c, true)
	if !ok {
		return
	}
	var req planRunRequest
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
	if content, cerr := trendlymodels.GetContent(brandID, contentID); cerr != nil || content == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such content"})
		return
	}

	// The money gate stays here. A 402 must arrive while the user is looking at
	// the button they pressed, with the reason that decides which prompt to
	// show — a top-up or a tier.
	model, blocked := gatePlanner(c, brandID)
	if blocked {
		return
	}

	// An empty model list is the NORMAL state of a workspace where nothing has
	// been enabled — `enabled` is a review gate. Said plainly rather than left
	// for the planner to discover, because a plan that can only reuse existing
	// media is a reasonable plan and the user should know that is why.
	if !anyGenerativeModelEnabled(ctx) {
		c.JSON(http.StatusOK, gin.H{
			"runId":   "",
			"message": "No AI models are switched on for your workspace, so a plan could only reuse media you already have. Ask us to enable them.",
		})
		return
	}

	// Checked BEFORE the row is written. With no queue the row would sit in
	// `planning` forever and the user would watch a spinner that never
	// resolves — which reads as slowness rather than breakage, and is the worst
	// failure shape available. Env vars are per function in
	// serverless.trendly.yml, so this is a real possibility, not a theoretical
	// one.
	if !studiojobs.Configured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "planner_unavailable",
			"message": "The director isn't available right now. Please try again shortly.",
		})
		return
	}

	// The model is stored on the run so the worker uses the one this request
	// gated on, rather than re-resolving it and possibly picking another.
	run := &trendlymodels.StudioRun{
		Status:    trendlymodels.RunPlanning,
		Mode:      req.Mode,
		Brief:     strings.TrimSpace(req.Brief),
		Model:     model,
		CreatedBy: c.GetString("userId"),
	}
	runID, err := trendlymodels.CreateStudioRun(ctx, brandID, contentID, studioID, run)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not start the plan"})
		return
	}

	if err := studiojobs.Enqueue(studiojobs.Message{
		Type:      studiojobs.JobDirectorPlan,
		BrandID:   brandID,
		ContentID: contentID,
		StudioID:  studioID,
		RunID:     runID,
	}); err != nil {
		// The row would otherwise be a permanent "planning". Marked failed so
		// the storyboard can say so and offer a retry.
		_ = trendlymodels.UpdateStudioRun(ctx, brandID, contentID, studioID, runID,
			[]firestore.Update{
				{Path: "status", Value: trendlymodels.RunFailed},
				{Path: "error", Value: "could not be queued"},
			})
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not start the plan"})
		return
	}

	// 202: accepted, not done. The client polls the run.
	c.JSON(http.StatusAccepted, gin.H{
		"runId":  runID,
		"status": trendlymodels.RunPlanning,
	})
}

// anyGenerativeModelEnabled reports whether anything can be generated at all.
//
// Cheaper than building a manifest just to count models, which is why the check
// is here and not behind BuildManifest: this runs on every plan request, and
// the manifest reads the design system and the media library too.
func anyGenerativeModelEnabled(ctx context.Context) bool {
	for _, m := range trendlymodels.CachedMediaModels(ctx) {
		if !m.Enabled {
			continue
		}
		switch m.Kind {
		case "video", "image", "music", "voice":
			return true
		}
	}
	return false
}

// ExecutePlanJob is the queued half: it does the planning and writes the answer
// onto the run row.
//
// Runs in studio_sqs, where the timeout is minutes rather than seconds. Every
// exit path updates the run — a row left in `planning` is indistinguishable
// from slowness, so "failed with a reason" is always better than silence.
func ExecutePlanJob(ctx context.Context, msg studiojobs.Message) error {
	brandID, contentID, studioID, runID := msg.BrandID, msg.ContentID, msg.StudioID, msg.RunID

	fail := func(reason string) error {
		return trendlymodels.UpdateStudioRun(ctx, brandID, contentID, studioID, runID,
			[]firestore.Update{
				{Path: "status", Value: trendlymodels.RunFailed},
				{Path: "error", Value: reason},
			})
	}

	run, err := trendlymodels.GetStudioRun(ctx, brandID, contentID, studioID, runID)
	if err != nil || run == nil {
		// Nothing to write to. Returning nil drops the message rather than
		// retrying forever against a row that does not exist.
		log.Printf("studiojobs: director_plan for a missing run %s/%s/%s/%s", brandID, contentID, studioID, runID)
		return nil
	}
	s, err := trendlymodels.GetStudio(ctx, brandID, contentID, studioID)
	if err != nil || s == nil {
		return fail("the studio was deleted")
	}
	content, err := trendlymodels.GetContent(brandID, contentID)
	if err != nil || content == nil {
		return fail("the content was deleted")
	}

	manifest, err := director.BuildManifest(ctx, brandID, content, s, run.Mode, 0)
	if err != nil {
		return fail("could not read what this brand has")
	}
	userMsg, err := director.BuildUserMessage(manifest, run.Brief)
	if err != nil {
		return fail("could not assemble the brief")
	}

	messages := []openrouter.Message{
		{Role: "system", Content: director.SystemPrompt},
		{Role: "user", Content: userMsg},
	}

	plan, problems, raw, perr := planOnce(ctx, brandID, run.Model, messages)
	if perr != nil {
		return fail("the director didn't answer")
	}
	// Fill the document voice BEFORE validating, so a plan is never refused
	// over a field the user cannot supply. Voice ids live in a provider
	// account; "pick one and plan again" asks for something nobody holds.
	if voiceID, supplied := plan.EnsureVoice(manifest); supplied {
		log.Printf("director: no voice in the plan, using %s for %s/%s", voiceID, brandID, studioID)
	}

	// Validated against the manifest the planner was GIVEN, not a freshly built
	// one: a wallet that moved between the call and the check would make a plan
	// fail validation for a reason that had nothing to do with it.
	if len(problems) == 0 {
		problems = plan.Validate(manifest)
	}

	// ONE repair round. The planner is told what it broke and asked to fix it.
	// One rather than a loop: in practice the second attempt either fixes it or
	// reveals a brief the rules genuinely forbid, and a loop spends money
	// discovering that slowly.
	if director.Fatal(problems) {
		messages = append(messages,
			openrouter.Message{Role: "assistant", Content: raw},
			openrouter.Message{Role: "user", Content: director.RepairMessage(problems)})
		if repaired, rproblems, rraw, rerr := planOnce(ctx, brandID, run.Model, messages); rerr == nil {
			// The repair round gets the same guarantee as the first.
			repaired.EnsureVoice(manifest)
			if len(rproblems) == 0 {
				rproblems = repaired.Validate(manifest)
			}
			plan, problems, raw = repaired, rproblems, rraw
		}
	}

	storyboard := director.Storyboard(plan, manifest, problems)
	_, derived := plan.DerivedCosts(manifest)

	// A plan that could never run is stored as REFUSED rather than discarded:
	// the problems are the useful part, and the user is entitled to see why
	// their brief was turned down.
	status := trendlymodels.RunPlanned
	if director.Fatal(problems) {
		status = trendlymodels.RunRefused
	}

	// ⚠️ EVERY ONE OF THESE IS MODEL OUTPUT. Firestore's protobuf refuses a
	// string field that is not valid UTF-8, and it refuses the WHOLE write —
	// so one truncated multi-byte sequence in the storyboard throws away the
	// plan as well. See safeForFirestore.
	if err := trendlymodels.UpdateStudioRun(ctx, brandID, contentID, studioID, runID,
		[]firestore.Update{
			{Path: "status", Value: status},
			{Path: "planJson", Value: safeForFirestore(raw)},
			{Path: "storyboard", Value: safeForFirestore(storyboard)},
			{Path: "problems", Value: flattenProblems(problems)},
			{Path: "estCostUsd", Value: derived},
		}); err != nil {
		// ⚠️ THE RUN MUST NOT BE LEFT `planning`. This is the call that
		// records the outcome, so its failure is the one failure that can
		// strand a run forever — the user watches "the director is still
		// working on this" against a job that finished minutes ago and will
		// never speak again. Say it failed, with the plain-ASCII reason that
		// could not possibly be what the first write choked on.
		log.Printf("director: could not store plan for run %s: %v", runID, err)
		return fail("we couldn't save that plan. Please try planning it again")
	}
	return nil
}

// planOnce makes one planner call and validates the answer.
func planOnce(
	ctx context.Context,
	brandID, model string,
	messages []openrouter.Message,
) (director.Plan, []director.Problem, string, error) {
	resp, err := openrouter.ChatCompletion(ctx, openrouter.ChatRequest{
		Model:    model,
		Messages: messages,
	})
	if err != nil {
		return director.Plan{}, nil, "", err
	}
	ai.MeterBrandUsage(brandID, resp.Usage)

	raw := cleanJSON(firstContent(resp))
	var plan director.Plan
	if uerr := json.Unmarshal([]byte(raw), &plan); uerr != nil {
		// Unparseable output is a problem like any other, so it goes through
		// the repair path rather than failing the request: the planner can
		// usually fix its own JSON when told what was wrong with it.
		return plan, []director.Problem{{
			Where:   "plan",
			Message: "the answer was not valid JSON: " + uerr.Error(),
			Fatal:   true,
		}}, raw, nil
	}
	// Validation happens in the caller, against the manifest it already holds.
	// Doing it here would mean either passing the manifest down for no other
	// reason or rebuilding it — and a rebuilt manifest can differ from the one
	// the planner was given, which would fail a plan for a reason that was not
	// its fault.
	return plan, nil, raw, nil
}

// GetRun returns one stored plan and its storyboard.
// GET …/studios/:studioId/runs/:runId
func GetRun(c *gin.Context) {
	brandID, contentID, studioID, ok := guard(c, false)
	if !ok {
		return
	}
	run, err := trendlymodels.GetStudioRun(c.Request.Context(), brandID, contentID, studioID, c.Param("runId"))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read the run"})
		return
	}
	if run == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such run"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"run": run})
}

// ListRuns returns a studio's runs, newest first.
// GET …/studios/:studioId/runs
func ListRuns(c *gin.Context) {
	brandID, contentID, studioID, ok := guard(c, false)
	if !ok {
		return
	}
	runs, err := trendlymodels.ListStudioRuns(c.Request.Context(), brandID, contentID, studioID, 20)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read the runs"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"runs": runs})
}

func flattenProblems(ps []director.Problem) []trendlymodels.RunProblem {
	out := make([]trendlymodels.RunProblem, 0, len(ps))
	for _, p := range ps {
		// A problem message quotes what the planner wrote — a scene id, a
		// model name, a fragment of its own JSON — so it carries the same
		// invalid-UTF-8 risk as the plan itself.
		out = append(out, trendlymodels.RunProblem{
			Field: safeForFirestore(p.Where), Message: safeForFirestore(p.Message), Fatal: p.Fatal,
		})
	}
	return out
}

// gatePlanner refuses when the wallet is empty or no reasoning model is
// unlocked, and returns the model to use.
//
// ⚠️ It tries TaskReasoning and falls back to TaskDesign. The reason is a trap
// worth naming: the model registry lives in the `ai_config` Firestore
// collection and REPLACES the built-in defaults, so a task with no seeded key
// resolves as locked for every plan — which would read to the user as "your
// plan doesn't include this" when nothing is wrong except a missing row. The
// fallback keeps planning working on a workspace whose config predates it.
func gatePlanner(c *gin.Context, brandID string) (model string, blocked bool) {
	orgID, _ := trendlymodels.OrgIDForBrand(brandID)
	if trendlymodels.TokensExhausted(orgID) {
		c.JSON(http.StatusPaymentRequired, gin.H{
			"error":   "upgrade_required",
			"reason":  "tokens_exhausted",
			"feature": "studio_director",
		})
		return "", true
	}

	ctx := c.Request.Context()
	for _, task := range []openrouter.TaskType{openrouter.TaskReasoning, openrouter.TaskDesign} {
		if m, locked := ai.PickModelFor(ctx, brandID, task); !locked && m != "" {
			return m, false
		}
	}
	c.JSON(http.StatusPaymentRequired, gin.H{
		"error":   "upgrade_required",
		"reason":  "plan_locked",
		"feature": "studio_director",
	})
	return "", true
}

// cleanJSON strips the markdown fence models add despite being told not to.
func cleanJSON(raw string) string {
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(s, "```") {
		if i := strings.Index(s, "\n"); i >= 0 {
			s = s[i+1:]
		}
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	}
	s = strings.TrimSpace(s)
	// Models occasionally wrap the object in prose. Finding the braces is
	// cheaper and more reliable than a second call to fix it.
	if i := strings.Index(s, "{"); i > 0 {
		if j := strings.LastIndex(s, "}"); j > i {
			s = s[i : j+1]
		}
	}
	return s
}
