package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// toolUpdateBrandMemory is the server tool, attached to EVERY chat module, that
// lets the AI persist durable brand facts into the brand's long-term memory.
const toolUpdateBrandMemory = "update_brand_memory"

// maxBrandMemoryChars caps the per-brand AI memory blob. When a merge would
// exceed this, the memory is compacted (deduped/summarised/pruned) by a cheap
// LLM call back under the cap rather than growing unbounded. PROVISIONAL value —
// tune once we see real memory sizes.
const maxBrandMemoryChars = 4000

// memoryInstructions tells the model it owns a long-term, per-brand memory and
// when to write to it. Appended to every module's system prompt (see
// buildSystemPrompt) so the capability exists in every conversation.
const memoryInstructions = "\n\nYou have a long-term memory for this brand (shown above as 'Brand memory' when it " +
	"is non-empty). Whenever the user tells you a durable, reusable fact about the brand — its positioning, target " +
	"audience, voice/tone, key products or services, do's and don'ts, hard constraints, or a recurring preference — " +
	"call the update_brand_memory tool with just that new fact so neither you nor future conversations have to ask " +
	"for it again. Never store one-off, transient, or task-specific details, and don't announce that you're saving to " +
	"memory unless the user asks."

// memoryServerTools is attached to EVERY module so the AI can persist durable
// brand facts from any conversation (general, strategy, calendar, content,
// onboarding). It is a looping server tool — it executes on the backend and the
// model continues its reply afterwards.
func memoryServerTools() []openrouter.Tool {
	return []openrouter.Tool{
		openrouter.NewFunctionTool(
			toolUpdateBrandMemory,
			"Save a durable, reusable fact about THIS brand to its long-term memory so future "+
				"conversations never have to ask for it again. Use it for lasting context — positioning, "+
				"target audience, brand voice/tone, key products or services, do's and don'ts, constraints, "+
				"or recurring preferences. Pass ONLY the new fact(s) in `memory`; do NOT restate the existing "+
				"memory — it is merged automatically. Do not call it for one-off or transient details.",
			openrouter.ObjectSchema(map[string]any{
				"memory": openrouter.StringProp("The new durable fact(s) about the brand to remember, as a short sentence or two."),
				"reason": openrouter.StringProp("Optional: a brief note on why this is worth remembering long-term."),
			}, []string{"memory"}),
		),
	}
}

type updateBrandMemoryArgs struct {
	Memory string `json:"memory"`
	Reason string `json:"reason"`
}

// updateBrandMemory merges a new fact into the brand's AI memory blob, compacting
// it with a cheap LLM pass when the merged result exceeds maxBrandMemoryChars.
// Returns a JSON result for the model; never terminal (the model keeps replying).
func updateBrandMemory(ctx context.Context, brandID, arguments string) (string, bool, error) {
	var a updateBrandMemoryArgs
	if arguments != "" {
		if err := json.Unmarshal([]byte(arguments), &a); err != nil {
			return jsonResult(map[string]any{"ok": false, "error": "could not parse arguments"}), false, nil
		}
	}
	fact := strings.TrimSpace(a.Memory)
	if fact == "" {
		return jsonResult(map[string]any{"ok": false, "error": "memory is required"}), false, nil
	}

	brand, err := loadBrand(brandID)
	if err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "brand not found"}), false, err
	}
	existing := ""
	if brand.AIMemory != nil {
		existing = strings.TrimSpace(*brand.AIMemory)
	}

	// Merge: append the new fact as its own bullet line so the blob stays a
	// readable list whether or not the user has hand-edited it.
	merged := "- " + fact
	if existing != "" {
		merged = existing + "\n- " + fact
	}

	compacted := false
	if len(merged) > maxBrandMemoryChars {
		if c, ok := compactBrandMemory(ctx, brandID, merged); ok {
			merged = c
		} else {
			// Compaction unavailable/failed — hard-trim so memory never grows
			// unbounded even when the LLM pass can't run.
			merged = truncateMemory(merged, maxBrandMemoryChars)
		}
		compacted = true
	}

	if err := trendlymodels.SetBrandMemory(ctx, brandID, merged); err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "failed to save: " + err.Error()}), false, err
	}
	return jsonResult(map[string]any{
		"ok":          true,
		"memoryChars": len(merged),
		"compacted":   compacted,
	}), false, nil
}

// compactBrandMemory asks a cheap model to rewrite the memory as a concise,
// de-duplicated bullet list under the cap, preserving the newest facts on
// conflict. Returns (compacted, true) on success; ("", false) when the model is
// unavailable/locked or the call fails, so the caller can fall back to a trim.
func compactBrandMemory(ctx context.Context, brandID, memory string) (string, bool) {
	model, locked := pickModel(ctx, brandID, openrouter.TaskChat, "")
	if locked || model == "" {
		return "", false
	}
	sys := "You compact a brand's long-term memory. Output ONLY the rewritten memory — no preamble, no commentary."
	user := fmt.Sprintf(
		"Rewrite the brand memory below as a concise, de-duplicated bulleted list of the most important DURABLE facts "+
			"about the brand. Merge overlapping points, drop stale, contradicted, trivial or one-off items, and keep the "+
			"most recent information when two points conflict. Keep it well under %d characters. Output only the bullet list.\n\n"+
			"Brand memory:\n%s",
		maxBrandMemoryChars, memory,
	)
	resp, err := openrouter.ChatCompletion(ctx, openrouter.ChatRequest{
		Model:    model,
		Messages: []openrouter.Message{{Role: "system", Content: sys}, {Role: "user", Content: user}},
	})
	if err != nil || len(resp.Choices) == 0 {
		return "", false
	}
	orgID, _ := orgIDForBrand(brandID)
	meterAIUsage(orgID, resp.Usage)

	out := strings.TrimSpace(resp.Choices[0].Message.Content)
	if out == "" {
		return "", false
	}
	// Safety net: if the model ignored the cap, hard-trim.
	if len(out) > maxBrandMemoryChars {
		out = truncateMemory(out, maxBrandMemoryChars)
	}
	return out, true
}

// truncateMemory hard-caps memory to n characters, preferring a line boundary so
// we don't cut a fact mid-sentence. Used only as a fallback when LLM compaction
// can't run.
func truncateMemory(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	if i := strings.LastIndex(s, "\n"); i > n/2 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
