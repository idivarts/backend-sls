// One-off: add the "design" task to a live ai_config/tasks document.
//
// Deliberately an Update on a single field path, not a Set: seed_ai_config
// replaces both documents wholesale, which would swap the deployed model ids
// for the ones this repo has drifted to. This touches tasks.design and nothing
// else.
//
//	go run ./scripts/add_design_task          # dry run
//	APPLY=1 go run ./scripts/add_design_task  # write
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"time"

	"cloud.google.com/go/firestore"
	_ "github.com/idivarts/backend-sls/pkg/firebase"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

// The deployed catalog's ids, best -> fallback. Every one must be
// vision-capable: the design loop renders its own output and looks at it.
var designAllowed = []string{
	"anthropic/claude-opus-4.8",
	"openai/gpt-5.5",
	"google/gemini-3-pro-preview",
	"openai/gpt-5.4",
	"google/gemini-3.5-flash",
}

func main() {
	ctx := context.Background()
	apply := os.Getenv("APPLY") == "1"

	db := os.Getenv("FIRESTORE_DATABASE_ID")
	if db == "" {
		db = "(default)"
	}
	fmt.Printf("database: %s\n", db)

	col := firestoredb.Client.Collection("ai_config")

	// Check every id against the catalog that is actually deployed, so this
	// cannot write a task pointing at a model that does not exist.
	msnap, err := col.Doc("models").Get(ctx)
	if err != nil {
		log.Fatalf("read ai_config/models: %v", err)
	}
	live := map[string]map[string]any{}
	if raw, ok := msnap.Data()["models"].([]any); ok {
		for _, m := range raw {
			if mm, ok := m.(map[string]any); ok {
				if id, ok := mm["id"].(string); ok {
					live[id] = mm
				}
			}
		}
	}
	bad := false
	fmt.Println("\nchecking the allow-list against the deployed catalog:")
	for _, id := range designAllowed {
		m, ok := live[id]
		switch {
		case !ok:
			fmt.Printf("  ✗ %-34s NOT IN CATALOG\n", id)
			bad = true
		case m["vision"] != true:
			fmt.Printf("  ✗ %-34s not vision-capable\n", id)
			bad = true
		default:
			fmt.Printf("  ✓ %-34s minPlan=%v enabled=%v\n", id, m["minPlan"], m["enabled"])
		}
	}
	if bad {
		log.Fatal("refusing to write: the allow-list does not match the deployed catalog")
	}

	tsnap, err := col.Doc("tasks").Get(ctx)
	if err != nil {
		log.Fatalf("read ai_config/tasks: %v", err)
	}
	tasks, _ := tsnap.Data()["tasks"].(map[string]any)
	keys := make([]string, 0, len(tasks))
	for k := range tasks {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Printf("\ntasks before (%d): %v\n", len(keys), keys)
	if existing, ok := tasks["design"]; ok {
		fmt.Printf("design present: %v\n", existing)
		verify(ctx)
		return
	}

	if !apply {
		fmt.Printf("\nDRY RUN — would set tasks.design.allowed = %v\n", designAllowed)
		fmt.Println("re-run with APPLY=1 to write.")
		return
	}

	_, err = col.Doc("tasks").Update(ctx, []firestore.Update{
		{FieldPath: firestore.FieldPath{"tasks", "design"}, Value: map[string]any{"allowed": designAllowed}},
		{Path: "updatedAt", Value: time.Now().UnixMilli()},
	})
	if err != nil {
		log.Fatalf("write failed: %v", err)
	}
	fmt.Println("\nwrote tasks.design")
}
