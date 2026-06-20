package ai

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// moduleStrategy is the AI module that powers the content-strategy editor. It
// runs against a single strategy doc (the conversation's ContextID is the
// strategyId) and uses the server tools below to collect a brief, generate the
// strategy document, and apply chat-driven edits to it.
const moduleStrategy = "strategy"

const dayMs = 86_400_000

const (
	toolSetStrategyBrief    = "set_strategy_brief"
	toolGenerateStrategyDoc = "generate_strategy_doc"
	toolApplyStrategyEdit   = "apply_strategy_edit"
)

// strategyInstructions is the conversational persona for the strategy module.
// The existing strategy doc (name/objective/markdownContent) is injected as
// module context above, so the model knows what — if anything — already exists.
const strategyInstructions = "\n\nYou help the user build and refine a content-strategy document. There are two phases.\n\n" +
	"PHASE 1 — CREATE (only when the document is still empty):\n" +
	"Collect, conversationally (NOT as a rigid step-by-step form, and skip anything that's obvious or already known): " +
	"(1) the target audience, (2) the core content pillars/topics, (3) the marketing funnel stage they're focused on " +
	"(awareness, consideration, conversion or loyalty — the strategy differs by stage), (4) the content types & formats, " +
	"(5) which platforms they'll post on, and (6) how long the strategy should run (1–6 months; gently steer toward 1 or 2 months " +
	"since longer plans are rarely useful).\n" +
	"- Keep it warm and natural; ask at most one or two things per message, and infer what you can instead of asking.\n" +
	"- For constrained answers (funnel stage, platforms, formats, duration) use the ask_options tool.\n" +
	"- As you learn the durable facts (name, objective, platforms, content formats, duration in days) call set_strategy_brief " +
	"with just those fields so they're saved.\n" +
	"- Once you have enough, call generate_strategy_doc with a complete strategy as HTML in markdownContent. The document must be " +
	"professional and DESCRIPTIVE AT THE IDEA LEVEL (not the individual-post level): a title, the audience & funnel framing, the " +
	"content pillars, and a plan broken down across the duration (e.g. by week/period) with concrete content ideas under each — " +
	"enough that it can later be expanded into a content calendar. Use semantic HTML (<h1>/<h2>/<h3>, <p>, <ul>/<li>, <strong>). " +
	"After it succeeds, send a short, warm closing message.\n\n" +
	"PHASE 2 — EDIT (when the document already has content):\n" +
	"Answer questions about the strategy, and when the user asks for a change — especially when they've focused on a specific " +
	"passage (shown above as focused text) — apply it with the apply_strategy_edit tool. Prefer mode 'replace_snippet' with the " +
	"smallest exact oldText that needs to change and its newText; fall back to mode 'replace_all' (sending the whole new HTML body) " +
	"only for sweeping rewrites. Never describe an edit you could make with the tool — make it, then briefly confirm what you changed."

// strategyServerTools are executed on the backend and only attached when the
// conversation's module is strategy.
func strategyServerTools() []openrouter.Tool {
	return []openrouter.Tool{
		openrouter.NewFunctionTool(
			toolSetStrategyBrief,
			"Save the durable facts of the strategy as you learn them during the create conversation. "+
				"Pass only the fields you just learned. Call it incrementally — it does not overwrite fields you omit.",
			openrouter.ObjectSchema(map[string]any{
				"name":           openrouter.StringProp("A short title for the strategy."),
				"objective":      openrouter.StringProp("The primary marketing goal (e.g. awareness, sales, engagement)."),
				"platforms":      arrayOfStrings("Platforms the strategy targets (e.g. Instagram, YouTube)."),
				"contentFormats": arrayOfStrings("Content formats planned (e.g. Reel, Story, Post, Carousel, Text Post)."),
				"durationDays":   openrouter.NumberProp("How many days the strategy runs (e.g. 30 for one month)."),
			}, []string{}),
		),
		openrouter.NewFunctionTool(
			toolGenerateStrategyDoc,
			"Write the full strategy document. Call this once enough of the brief is collected. "+
				"markdownContent must be complete, professional HTML. Returns the strategyId on success.",
			openrouter.ObjectSchema(map[string]any{
				"name":            openrouter.StringProp("The strategy title."),
				"objective":       openrouter.StringProp("The primary marketing goal."),
				"markdownContent": openrouter.StringProp("The complete strategy document as semantic HTML."),
				"durationDays":    openrouter.NumberProp("How many days the strategy runs."),
			}, []string{"markdownContent"}),
		),
		openrouter.NewFunctionTool(
			toolApplyStrategyEdit,
			"Apply an edit to the existing strategy document. Use mode 'replace_snippet' with an exact oldText and its "+
				"newText for targeted changes, or 'replace_all' with the full new HTML in newContent for a sweeping rewrite.",
			openrouter.ObjectSchema(map[string]any{
				"mode": openrouter.EnumProp(
					"'replace_snippet' to swap an exact passage, 'replace_all' to replace the whole body.",
					[]string{"replace_snippet", "replace_all"},
				),
				"oldText":    openrouter.StringProp("For replace_snippet: the exact existing text/HTML to replace."),
				"newText":    openrouter.StringProp("For replace_snippet: the replacement text/HTML."),
				"newContent": openrouter.StringProp("For replace_all: the complete new strategy HTML body."),
			}, []string{"mode"}),
		),
	}
}

func arrayOfStrings(description string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": description,
		"items":       map[string]any{"type": "string"},
	}
}

// dispatchStrategyTool runs a strategy server tool. Returns a JSON result string
// (fed back to the model), whether the strategy doc is now ready (true only on a
// successful generate_strategy_doc, so chat.go can emit `strategy_ready`), and
// any hard error.
func dispatchStrategyTool(ctx context.Context, brandID, strategyID, name, arguments string) (string, bool, error) {
	if strategyID == "" {
		return jsonResult(map[string]any{"ok": false, "error": "no strategy is associated with this conversation"}), false, nil
	}
	switch name {
	case toolSetStrategyBrief:
		return setStrategyBrief(ctx, brandID, strategyID, arguments)
	case toolGenerateStrategyDoc:
		return generateStrategyDoc(ctx, brandID, strategyID, arguments)
	case toolApplyStrategyEdit:
		return applyStrategyEdit(ctx, brandID, strategyID, arguments)
	default:
		return jsonResult(map[string]any{"ok": false, "error": "unknown strategy tool: " + name}), false, nil
	}
}

type setStrategyBriefArgs struct {
	Name           *string  `json:"name"`
	Objective      *string  `json:"objective"`
	Platforms      []string `json:"platforms"`
	ContentFormats []string `json:"contentFormats"`
	DurationDays   *float64 `json:"durationDays"`
}

func setStrategyBrief(ctx context.Context, brandID, strategyID, arguments string) (string, bool, error) {
	var a setStrategyBriefArgs
	if arguments != "" {
		if err := json.Unmarshal([]byte(arguments), &a); err != nil {
			return jsonResult(map[string]any{"ok": false, "error": "could not parse arguments"}), false, nil
		}
	}

	var updates []firestore.Update
	var saved []string
	if a.Name != nil {
		if v := strings.TrimSpace(*a.Name); v != "" {
			updates = append(updates, firestore.Update{Path: "name", Value: v})
			saved = append(saved, "name")
		}
	}
	if a.Objective != nil {
		updates = append(updates, firestore.Update{Path: "objective", Value: strings.TrimSpace(*a.Objective)})
		saved = append(saved, "objective")
	}
	if a.Platforms != nil {
		if v := cleanStrings(a.Platforms); len(v) > 0 {
			updates = append(updates, firestore.Update{Path: "platforms", Value: v})
			saved = append(saved, "platforms")
		}
	}
	if a.ContentFormats != nil {
		if v := cleanStrings(a.ContentFormats); len(v) > 0 {
			updates = append(updates, firestore.Update{Path: "contentFormats", Value: v})
			saved = append(saved, "contentFormats")
		}
	}
	if a.DurationDays != nil && *a.DurationDays > 0 {
		updates = append(updates, firestore.Update{Path: "timeline", Value: timelineFromDays(*a.DurationDays)})
		saved = append(saved, "durationDays")
	}

	if len(updates) > 0 {
		updates = append(updates, firestore.Update{Path: "updatedAt", Value: time.Now().UnixMilli()})
		if err := trendlymodels.UpdateStrategy(ctx, brandID, strategyID, updates); err != nil {
			return jsonResult(map[string]any{"ok": false, "error": "failed to save: " + err.Error()}), false, err
		}
	}
	return jsonResult(map[string]any{"ok": true, "saved": saved}), false, nil
}

type generateStrategyArgs struct {
	Name            *string  `json:"name"`
	Objective       *string  `json:"objective"`
	MarkdownContent string   `json:"markdownContent"`
	DurationDays    *float64 `json:"durationDays"`
}

func generateStrategyDoc(ctx context.Context, brandID, strategyID, arguments string) (string, bool, error) {
	var a generateStrategyArgs
	if err := json.Unmarshal([]byte(arguments), &a); err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "could not parse arguments"}), false, nil
	}
	if strings.TrimSpace(a.MarkdownContent) == "" {
		return jsonResult(map[string]any{"ok": false, "error": "markdownContent is required"}), false, nil
	}

	now := time.Now().UnixMilli()
	updates := []firestore.Update{
		{Path: "markdownContent", Value: a.MarkdownContent},
		{Path: "status", Value: "active"},
		{Path: "updatedAt", Value: now},
		{Path: "lastEditedAt", Value: now},
		// The doc was (re)written wholesale by the AI — invalidate the CRDT
		// baseline so the live web editor re-bootstraps from this HTML, and
		// bump the generation so any yupdates from the old generation that
		// haven't been pruned yet are ignored by the re-bootstrapped editor.
		{Path: "crdtInitialized", Value: false},
		{Path: "crdtGeneration", Value: firestore.Increment(1)},
	}
	if a.Name != nil && strings.TrimSpace(*a.Name) != "" {
		updates = append(updates, firestore.Update{Path: "name", Value: strings.TrimSpace(*a.Name)})
	}
	if a.Objective != nil && strings.TrimSpace(*a.Objective) != "" {
		updates = append(updates, firestore.Update{Path: "objective", Value: strings.TrimSpace(*a.Objective)})
	}
	if a.DurationDays != nil && *a.DurationDays > 0 {
		updates = append(updates, firestore.Update{Path: "timeline", Value: timelineFromDays(*a.DurationDays)})
	}

	if err := trendlymodels.UpdateStrategy(ctx, brandID, strategyID, updates); err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "failed to save: " + err.Error()}), false, err
	}
	trendlymodels.PruneStrategyYUpdates(ctx, brandID, strategyID)

	return jsonResult(map[string]any{"ok": true, "strategyId": strategyID}), true, nil
}

type applyStrategyEditArgs struct {
	Mode       string `json:"mode"`
	OldText    string `json:"oldText"`
	NewText    string `json:"newText"`
	NewContent string `json:"newContent"`
}

func applyStrategyEdit(ctx context.Context, brandID, strategyID, arguments string) (string, bool, error) {
	var a applyStrategyEditArgs
	if err := json.Unmarshal([]byte(arguments), &a); err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "could not parse arguments"}), false, nil
	}

	var newBody string
	switch a.Mode {
	case "replace_all":
		if strings.TrimSpace(a.NewContent) == "" {
			return jsonResult(map[string]any{"ok": false, "error": "newContent is required for replace_all"}), false, nil
		}
		newBody = a.NewContent
	case "replace_snippet":
		if a.OldText == "" {
			return jsonResult(map[string]any{"ok": false, "error": "oldText is required for replace_snippet"}), false, nil
		}
		// Read the full, untruncated body so we never apply edits against the
		// possibly-truncated copy injected into the prompt context.
		strat, err := trendlymodels.GetStrategy(ctx, brandID, strategyID)
		if err != nil {
			return jsonResult(map[string]any{"ok": false, "error": "strategy not found"}), false, err
		}
		current := strat.MarkdownContent
		if !strings.Contains(current, a.OldText) {
			return jsonResult(map[string]any{
				"ok":     false,
				"reason": "oldText not found verbatim in the document — retry with the exact text, or use mode 'replace_all'.",
			}), false, nil
		}
		newBody = strings.Replace(current, a.OldText, a.NewText, 1)
	default:
		return jsonResult(map[string]any{"ok": false, "error": "mode must be 'replace_snippet' or 'replace_all'"}), false, nil
	}

	now := time.Now().UnixMilli()
	updates := []firestore.Update{
		{Path: "markdownContent", Value: newBody},
		{Path: "updatedAt", Value: now},
		{Path: "lastEditedAt", Value: now},
		{Path: "crdtInitialized", Value: false},
		{Path: "crdtGeneration", Value: firestore.Increment(1)},
	}
	if err := trendlymodels.UpdateStrategy(ctx, brandID, strategyID, updates); err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "failed to save: " + err.Error()}), false, err
	}
	trendlymodels.PruneStrategyYUpdates(ctx, brandID, strategyID)

	return jsonResult(map[string]any{"ok": true}), false, nil
}

// timelineFromDays builds the strategy's timeline map. The startDate at creation
// is nominal (now) — the authoritative placement is chosen at push-to-calendar
// time; only the start→end span (the duration) is meaningful here.
func timelineFromDays(days float64) map[string]any {
	start := time.Now().UnixMilli()
	return map[string]any{
		"startDate": start,
		"endDate":   start + int64(days)*dayMs,
	}
}
