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
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
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

// PlanRun asks the director for a plan and stores it for approval.
// POST …/studios/:studioId/plan
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
	content, err := trendlymodels.GetContent(brandID, contentID)
	if err != nil || content == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such content"})
		return
	}

	model, blocked := gatePlanner(c, brandID)
	if blocked {
		return
	}

	manifest, err := director.BuildManifest(ctx, brandID, content, s, req.Mode, req.TargetSeconds)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read what this brand has"})
		return
	}
	// An empty model list is the NORMAL state of a workspace where nothing has
	// been enabled — `enabled` is a review gate. Said plainly rather than left
	// for the planner to discover, because a plan that can only reuse existing
	// media is a reasonable plan and the user should know that is why.
	if len(manifest.Models) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"runId":   "",
			"message": "No AI models are switched on for your workspace, so a plan could only reuse media you already have. Ask us to enable them.",
		})
		return
	}

	userMsg, err := director.BuildUserMessage(manifest, req.Brief)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not assemble the brief"})
		return
	}

	messages := []openrouter.Message{
		{Role: "system", Content: director.SystemPrompt},
		{Role: "user", Content: userMsg},
	}

	plan, problems, raw, perr := planOnce(ctx, brandID, model, messages)
	if perr != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "planning_failed",
			"message": "The director didn't answer. Please try again.",
		})
		return
	}
	// Validated against the manifest the planner was GIVEN, not a freshly
	// built one: a wallet that moved between the call and the check would make
	// a plan fail validation for a reason that had nothing to do with it.
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
		if repaired, rproblems, rraw, rerr := planOnce(ctx, brandID, model, messages); rerr == nil {
			if len(rproblems) == 0 {
				rproblems = repaired.Validate(manifest)
			}
			plan, problems, raw = repaired, rproblems, rraw
		}
	}

	storyboard := director.Storyboard(plan, manifest, problems)
	_, derived := plan.DerivedCosts(manifest)

	run := &trendlymodels.StudioRun{
		Status:     trendlymodels.RunPlanned,
		Mode:       manifest.Budget.Mode,
		Brief:      strings.TrimSpace(req.Brief),
		PlanJSON:   raw,
		Storyboard: storyboard,
		Problems:   flattenProblems(problems),
		EstCostUSD: derived,
		Model:      model,
		CreatedBy:  c.GetString("userId"),
	}
	// A plan that could never run is stored as REFUSED rather than discarded:
	// the problems are the useful part, and the user is entitled to see why
	// their brief was turned down.
	if director.Fatal(problems) {
		run.Status = trendlymodels.RunRefused
	}

	runID, err := trendlymodels.CreateStudioRun(ctx, brandID, contentID, studioID, run)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not store the plan"})
		return
	}

	statusCode := http.StatusOK
	if director.Fatal(problems) {
		// 422: the request was understood and the answer is a plan that cannot
		// be run. Not a 500 — nothing is broken — and not a 200, because the
		// client must not offer an Approve button for it.
		statusCode = http.StatusUnprocessableEntity
	}
	c.JSON(statusCode, gin.H{
		"runId":      runID,
		"status":     run.Status,
		"storyboard": storyboard,
		"problems":   run.Problems,
		"estCostUsd": derived,
		"plan":       json.RawMessage(raw),
	})
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
		out = append(out, trendlymodels.RunProblem{
			Field: p.Where, Message: p.Message, Fatal: p.Fatal,
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
