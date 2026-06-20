// Command cleanup-prod-firestore is a maintenance tool for the PRODUCTION
// Firestore database (database id "trendly-prod", project trendly-9ab99).
//
// ⚠️  It ALWAYS connects to production. The database id is hardcoded to
//     "trendly-prod" and is NOT taken from FIRESTORE_DATABASE_ID, so it can't
//     accidentally point at dev/default.
//
// It does two things:
//
//  1. Counts — lists every top-level collection + known subcollection groups
//     and prints document counts.
//  2. Cleanup — prunes unneeded data per the decision sheet in
//     .claude/plans/these-are-list-of-tranquil-cloud.md.
//
// Safety:
//   - DRY-RUN by default — prints "would delete / would keep" and writes nothing.
//   - APPLY=1 required to perform deletes.
//   - CONFIRM=DELETE-PROD also required (second token) before any write.
//
//	go run ./scripts/cleanup-prod-firestore                              # count + dry-run plan
//	APPLY=1 CONFIRM=DELETE-PROD go run ./scripts/cleanup-prod-firestore  # execute deletes
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"sync"

	"cloud.google.com/go/firestore"
	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
	firebaseapp "github.com/idivarts/backend-sls/pkg/firebase"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// prodDatabaseID is the production Firestore database id. Hardcoded on purpose
// so this script never touches dev/default no matter what env vars are set.
const prodDatabaseID = "trendly-prod"

// confirmToken is the second token (besides APPLY=1) required before any write.
const confirmToken = "DELETE-PROD"

// subcollectionGroups are the known subcollection ids in the data model. They
// don't appear in the root Collections() listing, so we count them across the
// whole database via CollectionGroup queries (counting only — deletion of
// subcollections happens via recursive delete of their parent docs).
var subcollectionGroups = []string{
	"members",        // brands/{id}/members
	"teams",          // brands/{id}/teams
	"applications",   // collaborations/{id}/applications
	"invitations",    // collaborations/{id}/invitations, users/{id}/invitations
	"orgMembers",     // organizations/{id}/orgMembers
	"notifications",  // {users|managers}/{id}/notifications
	"socials",        // users/{id}/socials
	"socialsPrivate", // users/{id}/socialsPrivate
	"socialAccounts", // users/{id}/socialAccounts, brands/{id}/socialAccounts
	"socialTokens",   // users/{id}/socialTokens, brands/{id}/socialTokens
}

// deleteAllCollections are wiped completely (every doc + all subcollections).
var deleteAllCollections = []string{
	"agency-hires",
	"ai_conversations",
	"cached",
	"scrapped-socials",
	"scrapped-socials-n8n",
	"shareLinks",
	"websockets",
}

type collCount struct {
	name  string
	count int64
	err   error
}

var apply bool

func main() {
	apply = os.Getenv("APPLY") == "1"
	confirmed := os.Getenv("CONFIRM") == confirmToken
	if apply && !confirmed {
		log.Fatalf("APPLY=1 set but CONFIRM=%s missing — refusing to write. Re-run with: APPLY=1 CONFIRM=%s", confirmToken, confirmToken)
	}
	mode := "DRY-RUN (no writes) — set APPLY=1 CONFIRM=" + confirmToken + " to execute"
	if apply {
		mode = "APPLY (DELETING from PRODUCTION)"
	}

	ctx := context.Background()

	log.Printf("=========================================================")
	log.Printf(" cleanup-prod-firestore")
	log.Printf(" project=%s  db=%s", firebaseapp.ProjectID, prodDatabaseID)
	log.Printf(" mode=%s", mode)
	log.Printf("=========================================================")

	client, err := firestore.NewClientWithDatabase(ctx, firebaseapp.ProjectID, prodDatabaseID, option.WithCredentialsFile(firebaseapp.ConfigFile))
	if err != nil {
		log.Fatalf("failed to create Firestore client: %v", err)
	}
	defer client.Close()

	// ---- Step 0: counts (always, read-only) ----
	reportCounts(ctx, client)

	// ---- Step 1: conditional cleanup in dependency order ----
	fmt.Printf("\n================ CLEANUP (%s) ================\n", mapMode())

	survivingCollabIDs, survivingBrandIDsFromCollabs := cleanCollaborations(ctx, client)
	survivingBrandIDs, keepManagerIDs := cleanBrands(ctx, client, survivingBrandIDsFromCollabs)
	cleanContracts(ctx, client, survivingCollabIDs)
	cleanManagers(ctx, client, keepManagerIDs)
	cleanOrganizations(ctx, client, survivingBrandIDs)

	// ---- Step 2: unconditional full wipes ----
	for _, name := range deleteAllCollections {
		deleteWholeCollection(ctx, client, name)
	}

	fmt.Printf("\n================ DONE (%s) ================\n", mapMode())
	if !apply {
		fmt.Printf("This was a DRY-RUN. Nothing was deleted.\n")
		fmt.Printf("To execute: APPLY=1 CONFIRM=%s go run ./scripts/cleanup-prod-firestore\n", confirmToken)
	}
}

func mapMode() string {
	if apply {
		return "APPLIED"
	}
	return "DRY-RUN"
}

// ----------------------------------------------------------------------------
// Counting
// ----------------------------------------------------------------------------

func reportCounts(ctx context.Context, client *firestore.Client) {
	var topLevel []collCount
	it := client.Collections(ctx)
	for {
		col, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Fatalf("failed to list top-level collections: %v", err)
		}
		n, err := countQuery(ctx, col.Query)
		topLevel = append(topLevel, collCount{name: col.ID, count: n, err: err})
	}
	sort.Slice(topLevel, func(i, j int) bool { return topLevel[i].name < topLevel[j].name })

	var subGroups []collCount
	for _, id := range subcollectionGroups {
		n, err := countQuery(ctx, client.CollectionGroup(id).Query)
		subGroups = append(subGroups, collCount{name: id, count: n, err: err})
	}

	fmt.Printf("\n================ PRODUCTION Firestore counts (db=%s) ================\n\n", prodDatabaseID)
	fmt.Println("Top-level collections:")
	var total int64
	printRows(topLevel, &total)
	fmt.Println("\nSubcollection groups (collection-group counts across the whole DB):")
	var subTotal int64
	printRows(subGroups, &subTotal)
	fmt.Printf("\n--------------------------------------------------------------------\n")
	fmt.Printf("Top-level documents:        %12d\n", total)
	fmt.Printf("Subcollection-group docs:   %12d\n", subTotal)
	fmt.Printf("Grand total (all counted):  %12d\n", total+subTotal)
	fmt.Printf("====================================================================\n")
}

// countQuery runs a COUNT aggregation over a query and returns the document count.
func countQuery(ctx context.Context, q firestore.Query) (int64, error) {
	res, err := q.NewAggregationQuery().WithCount("all").Get(ctx)
	if err != nil {
		return 0, err
	}
	v, ok := res["all"]
	if !ok {
		return 0, fmt.Errorf("count alias 'all' missing from aggregation result")
	}
	cv, ok := v.(*pb.Value)
	if !ok {
		return 0, fmt.Errorf("unexpected aggregation value type %T", v)
	}
	return cv.GetIntegerValue(), nil
}

func printRows(rows []collCount, total *int64) {
	for _, r := range rows {
		if r.err != nil {
			fmt.Printf("  %-28s  ERROR: %v\n", r.name, r.err)
			continue
		}
		*total += r.count
		fmt.Printf("  %-28s  %12d\n", r.name, r.count)
	}
}

// ----------------------------------------------------------------------------
// Conditional cleanups (dependency order)
// ----------------------------------------------------------------------------

// cleanCollaborations keeps only docs with isLive == true; everything else
// (incl. missing isLive) is recursively deleted. Returns the surviving
// collaboration IDs and the set of brandIds referenced by surviving collabs.
func cleanCollaborations(ctx context.Context, client *firestore.Client) (survivingCollabIDs, survivingBrandIDs map[string]bool) {
	survivingCollabIDs = map[string]bool{}
	survivingBrandIDs = map[string]bool{}
	var toDelete []*firestore.DocumentRef
	var kept int

	iter := client.Collection("collaborations").Documents(ctx)
	defer iter.Stop()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Fatalf("collaborations iteration error: %v", err)
		}
		isLive, _ := doc.Data()["isLive"].(bool)
		if isLive {
			kept++
			survivingCollabIDs[doc.Ref.ID] = true
			if brandID, ok := doc.Data()["brandId"].(string); ok && brandID != "" {
				survivingBrandIDs[brandID] = true
			}
			continue
		}
		toDelete = append(toDelete, doc.Ref)
	}

	fmt.Printf("\n[collaborations] keep %d (isLive==true), delete %d (isLive!=true incl. missing)\n", kept, len(toDelete))
	recursiveDeleteDocs(ctx, client, toDelete, "collaborations")
	return survivingCollabIDs, survivingBrandIDs
}

// cleanBrands keeps only brands referenced by a surviving collaboration's
// brandId. Before deleting, it collects manager IDs from the members
// subcollection of each SURVIVING brand. Returns the surviving brand IDs and
// the set of manager IDs to keep.
func cleanBrands(ctx context.Context, client *firestore.Client, survivingBrandIDsFromCollabs map[string]bool) (survivingBrandIDs, keepManagerIDs map[string]bool) {
	survivingBrandIDs = map[string]bool{}
	keepManagerIDs = map[string]bool{}
	var toDelete []*firestore.DocumentRef
	var survivors []*firestore.DocumentRef

	iter := client.Collection("brands").Documents(ctx)
	defer iter.Stop()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Fatalf("brands iteration error: %v", err)
		}
		if survivingBrandIDsFromCollabs[doc.Ref.ID] {
			survivingBrandIDs[doc.Ref.ID] = true
			survivors = append(survivors, doc.Ref)
			continue
		}
		toDelete = append(toDelete, doc.Ref)
	}

	// Collect manager IDs from the members subcollection of surviving brands.
	// The members doc ID IS the managerId.
	for _, ref := range survivors {
		mIter := ref.Collection("members").Documents(ctx)
		for {
			mDoc, err := mIter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				log.Fatalf("members iteration error for brand %s: %v", ref.ID, err)
			}
			keepManagerIDs[mDoc.Ref.ID] = true
		}
		mIter.Stop()
	}

	fmt.Printf("\n[brands] keep %d (referenced by surviving collabs), delete %d; managerIds to keep: %d\n",
		len(survivingBrandIDs), len(toDelete), len(keepManagerIDs))
	recursiveDeleteDocs(ctx, client, toDelete, "brands")
	return survivingBrandIDs, keepManagerIDs
}

// cleanContracts keeps contracts whose collaborationId is empty OR references a
// surviving collaboration; deletes the rest.
func cleanContracts(ctx context.Context, client *firestore.Client, survivingCollabIDs map[string]bool) {
	var toDelete []*firestore.DocumentRef
	var kept int

	iter := client.Collection("contracts").Documents(ctx)
	defer iter.Stop()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Fatalf("contracts iteration error: %v", err)
		}
		collabID, _ := doc.Data()["collaborationId"].(string)
		if collabID == "" || survivingCollabIDs[collabID] {
			kept++
			continue
		}
		toDelete = append(toDelete, doc.Ref)
	}

	fmt.Printf("\n[contracts] keep %d (empty or surviving collaborationId), delete %d\n", kept, len(toDelete))
	recursiveDeleteDocs(ctx, client, toDelete, "contracts")
}

// cleanManagers keeps managers whose ID is a member of a surviving brand;
// deletes the rest.
func cleanManagers(ctx context.Context, client *firestore.Client, keepManagerIDs map[string]bool) {
	var toDelete []*firestore.DocumentRef
	var kept int

	iter := client.Collection("managers").Documents(ctx)
	defer iter.Stop()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Fatalf("managers iteration error: %v", err)
		}
		if keepManagerIDs[doc.Ref.ID] {
			kept++
			continue
		}
		toDelete = append(toDelete, doc.Ref)
	}

	fmt.Printf("\n[managers] keep %d (member of a surviving brand), delete %d\n", kept, len(toDelete))
	recursiveDeleteDocs(ctx, client, toDelete, "managers")
}

// cleanOrganizations keeps orgs whose brandIds intersect the surviving brand
// set; deletes the rest.
func cleanOrganizations(ctx context.Context, client *firestore.Client, survivingBrandIDs map[string]bool) {
	var toDelete []*firestore.DocumentRef
	var kept int

	iter := client.Collection("organizations").Documents(ctx)
	defer iter.Stop()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Fatalf("organizations iteration error: %v", err)
		}
		if orgHasSurvivingBrand(doc, survivingBrandIDs) {
			kept++
			continue
		}
		toDelete = append(toDelete, doc.Ref)
	}

	fmt.Printf("\n[organizations] keep %d (has a surviving brand), delete %d\n", kept, len(toDelete))
	recursiveDeleteDocs(ctx, client, toDelete, "organizations")
}

func orgHasSurvivingBrand(doc *firestore.DocumentSnapshot, survivingBrandIDs map[string]bool) bool {
	raw, ok := doc.Data()["brandIds"]
	if !ok {
		return false
	}
	list, ok := raw.([]interface{})
	if !ok {
		return false
	}
	for _, v := range list {
		if id, ok := v.(string); ok && survivingBrandIDs[id] {
			return true
		}
	}
	return false
}

// ----------------------------------------------------------------------------
// Deletion helpers (recursive — the Go SDK has no RecursiveDelete)
// ----------------------------------------------------------------------------

// discoverWorkers bounds how many subtrees we walk concurrently. Discovery
// (listing subcollections per doc) is round-trip bound, so concurrency is the
// main speedup. BulkWriter enqueue stays single-goroutine (it is NOT safe for
// concurrent Delete calls).
const discoverWorkers = 24

// deleteWholeCollection recursively deletes every document (and all nested
// subcollections) of a top-level collection. Uses DocumentRefs (ListDocuments)
// so it is cheap even for large flat collections and also catches "missing"
// parent docs that exist only to hold subcollections.
func deleteWholeCollection(ctx context.Context, client *firestore.Client, name string) {
	refs, err := client.Collection(name).DocumentRefs(ctx).GetAll()
	if err != nil {
		log.Fatalf("%s ref listing error: %v", name, err)
	}
	fmt.Printf("\n[%s] DELETE ALL — %d top-level docs\n", name, len(refs))
	recursiveDeleteDocs(ctx, client, refs, name)
}

// recursiveDeleteDocs deletes each doc ref and all of its nested subcollections.
//
// Dry-run: prints the number of top-level docs that would be deleted (their
// subcollections go with them) without walking the tree — instant, no extra
// reads. The top-of-run count report already shows subcollection-group totals.
//
// Apply: discovers the full document tree in parallel, then enqueues every
// delete on a single BulkWriter (which batches the network writes internally).
func recursiveDeleteDocs(ctx context.Context, client *firestore.Client, refs []*firestore.DocumentRef, label string) {
	if len(refs) == 0 {
		return
	}
	if !apply {
		fmt.Printf("  [%s] DRY-RUN: would delete %d top-level docs (+ all their subcollections)\n", label, len(refs))
		return
	}

	all := collectTrees(ctx, refs)
	bw := client.BulkWriter(ctx)
	for i, ref := range all {
		if _, err := bw.Delete(ref); err != nil {
			log.Fatalf("failed to enqueue delete for %s: %v", ref.Path, err)
		}
		if (i+1)%2000 == 0 {
			log.Printf("  [%s] enqueued %d/%d ...", label, i+1, len(all))
		}
	}
	bw.End()
	fmt.Printf("  [%s] APPLIED: deleted %d docs (incl. subcollections)\n", label, len(all))
}

// collectTrees walks the given top-level refs (and all descendants) using a
// bounded worker pool and returns every doc ref in the forest.
func collectTrees(ctx context.Context, roots []*firestore.DocumentRef) []*firestore.DocumentRef {
	jobs := make(chan *firestore.DocumentRef)
	out := make(chan []*firestore.DocumentRef)

	var wg sync.WaitGroup
	for w := 0; w < discoverWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ref := range jobs {
				out <- collectOneTree(ctx, ref)
			}
		}()
	}
	go func() {
		for _, r := range roots {
			jobs <- r
		}
		close(jobs)
	}()
	go func() {
		wg.Wait()
		close(out)
	}()

	var all []*firestore.DocumentRef
	for s := range out {
		all = append(all, s...)
	}
	return all
}

// collectOneTree returns ref plus every descendant doc ref (depth-first,
// sequential within this subtree). Uses DocumentRefs so it lists doc refs
// without reading their contents.
func collectOneTree(ctx context.Context, ref *firestore.DocumentRef) []*firestore.DocumentRef {
	res := []*firestore.DocumentRef{ref}
	cols := ref.Collections(ctx)
	for {
		col, err := cols.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Fatalf("subcollection listing error for %s: %v", ref.Path, err)
		}
		children, err := col.DocumentRefs(ctx).GetAll()
		if err != nil {
			log.Fatalf("subcollection ref listing error under %s: %v", col.Path, err)
		}
		for _, child := range children {
			res = append(res, collectOneTree(ctx, child)...)
		}
	}
	return res
}
