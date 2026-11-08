package audit

// The checks that keep Firestore rules and indexes honest.
//
// These are the cheap, mechanical half of the standing rule. They cannot tell
// whether a NEW collection has a `match` block — that needs to know what the
// code queries — but they catch the three failures that have actually happened:
//
//  1. The two index files drifting apart. A query indexed only in
//     `firestore.indexes.json` hard-errors on dev; one indexed only in
//     `firestore.indexes.enterprise.json` fails on dev and SILENTLY FULL-SCANS
//     on prod. The second is worse, because nothing fails — the product just
//     gets slower as the collection grows.
//  2. `fieldOverrides` reaching the Enterprise file. Enterprise REJECTS any
//     field-index configuration on deploy (HTTP 400), so one entry there
//     fails the whole production index deploy — including the composite
//     indexes that were fine.
//  3. A raw Firestore client outside the model package.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type indexFile struct {
	Indexes        []map[string]any `json:"indexes"`
	FieldOverrides []map[string]any `json:"fieldOverrides"`
}

func loadIndexFile(t *testing.T, path string) indexFile {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var out indexFile
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return out
}

// The composite arrays must be IDENTICAL. Dev runs a Standard database and
// prod an Enterprise one, and the composite indexes a query needs are the same
// either way — only the single-field configuration differs.
func TestCompositeIndexesMatchAcrossEditions(t *testing.T) {
	root := repoRoot(t)
	dir := filepath.Join(root, "firestore", "trendly")

	std := loadIndexFile(t, filepath.Join(dir, "firestore.indexes.json"))
	ent := loadIndexFile(t, filepath.Join(dir, "firestore.indexes.enterprise.json"))

	if len(std.Indexes) != len(ent.Indexes) {
		t.Fatalf(`the two index files disagree: %d composite indexes in firestore.indexes.json,
%d in firestore.indexes.enterprise.json.

Every composite index belongs in BOTH. One present only in the Standard file
hard-errors on dev; one present only in the Enterprise file fails on dev AND
silently full-scans on production, which nothing reports.`,
			len(std.Indexes), len(ent.Indexes))
	}

	// Compared as canonical JSON rather than field by field: ordering inside
	// an index matters (it is a prefix), so a reordered copy is a different
	// index and should fail here.
	for i := range std.Indexes {
		a, _ := json.Marshal(std.Indexes[i])
		b, _ := json.Marshal(ent.Indexes[i])
		if string(a) != string(b) {
			t.Errorf("index %d differs between the editions:\n  standard:   %s\n  enterprise: %s", i, a, b)
		}
	}
}

// Enterprise rejects field-index configuration outright, so the file must carry
// none. A single-field index for production goes in a migrations/*.sh script as
// a `gcloud firestore indexes composite create` line instead.
func TestEnterpriseIndexFileHasNoFieldOverrides(t *testing.T) {
	root := repoRoot(t)
	ent := loadIndexFile(t, filepath.Join(root, "firestore", "trendly", "firestore.indexes.enterprise.json"))

	if len(ent.FieldOverrides) > 0 {
		t.Fatalf(`firestore.indexes.enterprise.json carries %d fieldOverrides.

Enterprise refuses the whole deploy with

  HTTP 400: Enterprise Edition does not support updating field index configuration

so these also take the composite indexes down with them. Keep the array empty
and add a gcloud line to firestore/trendly/migrations/ instead.`,
			len(ent.FieldOverrides))
	}
}

// Every collection the rules file mentions should be reachable; more usefully,
// the collections this rebuild introduced must have a block at all. A new
// collection with no `match` is readable by nobody or — worse, under a
// permissive parent — by everybody.
func TestNewCollectionsHaveRules(t *testing.T) {
	root := repoRoot(t)
	body, err := os.ReadFile(filepath.Join(root, "firestore", "trendly", "firestore.rules"))
	if err != nil {
		t.Fatalf("reading the rules: %v", err)
	}
	rules := string(body)

	// Each entry is a collection this codebase writes and the match block that
	// must exist for it. Adding a collection without adding it here is the gap
	// this test cannot see, which is why the list is explicit.
	required := map[string]string{
		"studios":    "match /studios/{studioId}",
		"scenes":     "match /scenes/{sceneId}",
		"renders":    "match /renders/{renderId}",
		"runs":       "match /runs/{runId}",
		"aggregates": "match /aggregates/{name}",
		"assets":     "match /assets/{assetId}",
	}
	for name, block := range required {
		if !strings.Contains(rules, block) {
			t.Errorf("the %q collection has no rules block (%q missing)", name, block)
		}
	}
}

// firestoredb.Client may only appear inside a model package.
//
// The standing rule, as a test — but as a RATCHET rather than a cliff. There
// are two dozen pre-existing callers in this codebase, most of them older than
// the rule, and failing the build on them would mean every CI run red until
// somebody refactors handlers unrelated to whatever they were working on. A
// test nobody can make pass gets deleted, and then the rule has nothing at all
// behind it.
//
// So: known offenders are listed, and the test fails only on a NEW one. The
// list may only ever SHRINK — removing a name is the reward for fixing it, and
// adding one is not an option, which is the whole point.
//
// The rule itself matters because a handler that queries Firestore directly is
// a handler whose query nobody indexed, whose fields nobody validated, and
// whose side effects — reference counts, storage rollups — nobody moved.
var firestoreClientDebt = map[string]bool{
	"functions/trendly_analytics_snapshot/main.go":    true,
	"functions/trendly_social_refresh/main.go":        true,
	"internal/ccapis/campaigns/create.go":             true,
	"internal/message_webhook/sqs/test.go":            true,
	"internal/middlewares/crowdychat_mw.go":           true,
	"internal/middlewares/trendly_mw.go":              true,
	"internal/trendly_discovery/discovery_test.go":    true,
	"internal/trendlyapis/brand.go":                   true,
	"internal/trendlyapis/chat.go":                    true,
	"internal/trendlyapis/monetize/dispute.go":        true,
	"internal/trendlyapis/monetize/webhook_routes.go": true,
	"internal/trendlyapis/organization.go":            true,
	"internal/trendlyapis/sla/sla_monitor.go":         true,
}

func TestFirestoreClientOnlyInModels(t *testing.T) {
	root := repoRoot(t)
	// Model packages own their collections' access, so all of them are
	// allowed — not just trendlymodels. The CrowdyChat models and the
	// BigQuery ones are model layers too.
	allowedPrefixes := []string{
		"internal/models/",
		// One-off migrations, backfills and seeds. They are not serving code:
		// each runs once, by hand, against a database somebody is watching.
		// Requiring a model function for a one-time backfill is friction with
		// no safety behind it — nothing downstream depends on a script's query
		// shape, and nobody's reference count moves because of it.
		"scripts/",
		// The openrouter registry reads its own config collection and is
		// already encapsulated; CLAUDE.md records it as the one exception.
		"pkg/openrouter/",
		"pkg/firebase/",
		"internal/audit/",
	}

	var offenders []string
	var fixed []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			switch info.Name() {
			case "node_modules", ".git", "dist", "vendor", "services":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		for _, p := range allowedPrefixes {
			if strings.HasPrefix(rel, p) {
				return nil
			}
		}
		body, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		uses := strings.Contains(string(body), "firestoredb.Client")
		if uses && !firestoreClientDebt[rel] {
			offenders = append(offenders, rel)
		}
		if !uses && firestoreClientDebt[rel] {
			fixed = append(fixed, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the tree: %v", err)
	}

	if len(offenders) > 0 {
		t.Errorf(`these files use firestoredb.Client outside a model package:

  %s

Add the missing function to the collection's model in
internal/models/trendlymodels/ and call that instead. A direct query is a query
nobody indexed and whose side effects — reference counts, rollups — nobody
moved.`, strings.Join(offenders, "\n  "))
	}

	// A fixed file left in the list is not a failure — it is the one direction
	// this list is allowed to move — but it should be taken out, or the list
	// stops meaning anything.
	if len(fixed) > 0 {
		t.Logf("these no longer use firestoredb.Client and can come off the debt list:\n  %s",
			strings.Join(fixed, "\n  "))
	}
}
