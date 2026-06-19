// Command seed_ai_config (re)seeds the Firestore AI model registry read by both
// the backend (pkg/openrouter, cached) and the brand app (directly, via the
// ai-config provider). It writes two documents:
//
//   - ai_config/models : the model catalog (id, displayName, provider, minPlan,
//     vision, imageGen, enabled, order)
//   - ai_config/tasks  : per-task ordered allowed-model lists (best -> fallbacks)
//
// Plans: free < pro < team < agency. A task whose allowed list contains no
// free-tier model is intentionally premium-only (script, image, reasoning) — the
// apps show an upgrade prompt for those on free.
//
// After seeding you can hand-edit either document in the Firebase console; the
// backend picks up changes within its cache TTL (~5 min) and the app live-updates
// via its Firestore subscription. Keep this in sync with
// pkg/openrouter.defaultRegistry (the built-in fallback used when Firestore is
// unavailable).
//
//	go run ./scripts/seed_ai_config          # dry-run (prints, no writes)
//	APPLY=1 go run ./scripts/seed_ai_config  # apply
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"time"

	_ "github.com/idivarts/backend-sls/pkg/firebase"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

func main() {
	apply := os.Getenv("APPLY") == "1"
	mode := "DRY-RUN (no writes) — set APPLY=1 to apply"
	if apply {
		mode = "APPLY (writing changes)"
	}
	log.Printf("seed_ai_config starting — %s", mode)

	now := time.Now().UnixMilli()

	models := map[string]any{
		"updatedAt": now,
		"models": []map[string]any{
			{"id": "google/gemini-3.5-flash", "displayName": "Gemini 3.5 Flash", "provider": "Google", "minPlan": "free", "vision": true, "imageGen": false, "enabled": true, "order": 0},
			{"id": "anthropic/claude-sonnet-4.6", "displayName": "Claude Sonnet 4.6", "provider": "Anthropic", "minPlan": "free", "vision": false, "imageGen": false, "enabled": true, "order": 1},
			{"id": "openai/gpt-5.4", "displayName": "GPT-5.4", "provider": "OpenAI", "minPlan": "pro", "vision": true, "imageGen": false, "enabled": true, "order": 2},
			{"id": "anthropic/claude-opus-4.8", "displayName": "Claude Opus 4.8", "provider": "Anthropic", "minPlan": "pro", "vision": true, "imageGen": false, "enabled": true, "order": 3},
			{"id": "google/gemini-3.1-pro-preview", "displayName": "Gemini 3 Pro", "provider": "Google", "minPlan": "team", "vision": true, "imageGen": false, "enabled": true, "order": 4},
			{"id": "openai/gpt-5.5", "displayName": "GPT-5.5", "provider": "OpenAI", "minPlan": "team", "vision": true, "imageGen": false, "enabled": true, "order": 5},
			{"id": "google/gemini-3.1-flash-image", "displayName": "Gemini 3.1 Flash Image", "provider": "Google", "minPlan": "pro", "vision": true, "imageGen": true, "enabled": true, "order": 6},
			{"id": "google/gemini-3-pro-image", "displayName": "Gemini 3 Pro Image", "provider": "Google", "minPlan": "team", "vision": true, "imageGen": true, "enabled": true, "order": 7},
		},
	}

	tasks := map[string]any{
		"updatedAt": now,
		"tasks": map[string]any{
			"chat":       map[string]any{"allowed": []string{"google/gemini-3.5-flash", "anthropic/claude-sonnet-4.6", "openai/gpt-5.4", "anthropic/claude-opus-4.8", "openai/gpt-5.5", "google/gemini-3.1-pro-preview"}},
			"quick_edit": map[string]any{"allowed": []string{"google/gemini-3.5-flash", "anthropic/claude-sonnet-4.6"}},
			"caption":    map[string]any{"allowed": []string{"google/gemini-3.5-flash", "anthropic/claude-sonnet-4.6", "openai/gpt-5.4"}},
			"hashtag":    map[string]any{"allowed": []string{"google/gemini-3.5-flash", "anthropic/claude-sonnet-4.6"}},
			"strategy":   map[string]any{"allowed": []string{"anthropic/claude-opus-4.8", "openai/gpt-5.5", "anthropic/claude-sonnet-4.6", "google/gemini-3.5-flash"}},
			"script":     map[string]any{"allowed": []string{"anthropic/claude-opus-4.8", "openai/gpt-5.5", "openai/gpt-5.4"}},
			"multimodal": map[string]any{"allowed": []string{"google/gemini-3.1-pro-preview", "openai/gpt-5.4", "google/gemini-3.5-flash"}},
			"reasoning":  map[string]any{"allowed": []string{"openai/gpt-5.5", "anthropic/claude-opus-4.8", "openai/gpt-5.4"}},
			"image":      map[string]any{"allowed": []string{"google/gemini-3-pro-image", "google/gemini-3.1-flash-image"}},
		},
	}

	if !apply {
		printJSON("ai_config/models", models)
		printJSON("ai_config/tasks", tasks)
		log.Printf("dry-run complete — set APPLY=1 to write")
		return
	}

	ctx := context.Background()
	if _, err := firestoredb.Client.Collection("ai_config").Doc("models").Set(ctx, models); err != nil {
		log.Fatalf("write ai_config/models: %v", err)
	}
	log.Printf("wrote ai_config/models (%d models)", len(models["models"].([]map[string]any)))
	if _, err := firestoredb.Client.Collection("ai_config").Doc("tasks").Set(ctx, tasks); err != nil {
		log.Fatalf("write ai_config/tasks: %v", err)
	}
	log.Printf("wrote ai_config/tasks (%d tasks)", len(tasks["tasks"].(map[string]any)))
	log.Printf("seed_ai_config done")
}

func printJSON(label string, v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	log.Printf("%s =\n%s", label, string(b))
}
