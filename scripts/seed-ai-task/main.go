// Command seed-ai-task adds (or updates) one task entry in ai_config/tasks.
//
// The live ai_config REPLACES the built-in model registry, so a task added in
// code but never seeded runs on the compiled-in fallback. Resolve handles that
// deliberately — it logs and falls back rather than telling the user to buy a
// plan that cannot help — but it leaves Firestore no longer authoritative,
// and the next person to read ai_config sees an incomplete picture.
//
// Dev and prod are different databases on the same project, so this is run
// twice with different FIRESTORE_DATABASE_ID values. That is the whole reason
// it is a committed script rather than a console edit: the prod run has to be
// the same operation, not a reconstruction of it.
//
//	# dev
//	go run ./scripts/seed-ai-task -task score \
//	    -models google/gemini-3.5-flash,anthropic/claude-sonnet-4.6,openai/gpt-5.4
//
//	# prod
//	FIRESTORE_DATABASE_ID=trendly-prod go run ./scripts/seed-ai-task -task score \
//	    -models google/gemini-3.5-flash,anthropic/claude-sonnet-4.6,openai/gpt-5.4
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

func main() {
	task := flag.String("task", "", "task key, e.g. score")
	models := flag.String("models", "", "comma-separated model ids, best first")
	dryRun := flag.Bool("dry-run", false, "print what would change and exit")
	flag.Parse()

	if *task == "" || *models == "" {
		log.Fatal("both -task and -models are required")
	}
	allowed := strings.Split(*models, ",")
	for i := range allowed {
		allowed[i] = strings.TrimSpace(allowed[i])
	}

	db := os.Getenv("FIRESTORE_DATABASE_ID")
	if db == "" {
		db = "(default)"
	}
	ctx := context.Background()
	ref := firestoredb.Client.Collection("ai_config").Doc("tasks")

	snap, err := ref.Get(ctx)
	if err != nil {
		log.Fatalf("read ai_config/tasks: %v", err)
	}
	existing, _ := snap.Data()["tasks"].(map[string]interface{})
	if existing == nil {
		existing = map[string]interface{}{}
	}

	verb := "adding"
	if existing[*task] != nil {
		verb = "REPLACING"
	}
	log.Printf("database %s: %s task %q with %v", db, verb, *task, allowed)
	if *dryRun {
		log.Print("dry run, nothing written")
		return
	}

	// Merge into the map rather than rewriting it: every other task's config
	// is live, and a full overwrite here would be a very quiet outage.
	if _, err := ref.Update(ctx, []firestore.Update{
		{Path: "tasks." + *task, Value: map[string]interface{}{"allowed": allowed}},
		{Path: "updatedAt", Value: time.Now().UnixMilli()},
	}); err != nil {
		log.Fatalf("update: %v", err)
	}
	log.Printf("done")
}
