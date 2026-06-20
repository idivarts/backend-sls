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

// deleteWholeCollection recursively deletes every document (and all nested
// subcollections) of a top-level collection.
func deleteWholeCollection(ctx context.Context, client *firestore.Client, name string) {
	var refs []*firestore.DocumentRef
	iter := client.Collection(name).Documents(ctx)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Fatalf("%s iteration error: %v", name, err)
		}
		refs = append(refs, doc.Ref)
	}
	iter.Stop()
	fmt.Printf("\n[%s] DELETE ALL — %d top-level docs\n", name, len(refs))
	recursiveDeleteDocs(ctx, client, refs, name)
}

// recursiveDeleteDocs deletes each doc ref and all of its nested subcollections.
// In dry-run it only counts (including subcollection docs).
func recursiveDeleteDocs(ctx context.Context, client *firestore.Client, refs []*firestore.DocumentRef, label string) {
	if len(refs) == 0 {
		return
	}
	if !apply {
		total := 0
		for _, ref := range refs {
			total += countDocTree(ctx, ref)
		}
		fmt.Printf("  [%s] DRY-RUN: would delete %d docs (incl. subcollections)\n", label, total)
		return
	}

	bw := client.BulkWriter(ctx)
	deleted := 0
	for _, ref := range refs {
		deleted += deleteDocTree(ctx, client, bw, ref)
		if deleted > 0 && deleted%500 == 0 {
			bw.Flush()
			log.Printf("  [%s] deleted %d docs...", label, deleted)
		}
	}
	bw.End()
	fmt.Printf("  [%s] APPLIED: deleted %d docs (incl. subcollections)\n", label, deleted)
}

// deleteDocTree recursively enqueues deletes for a doc's subcollections (depth
// first) then the doc itself, on the given BulkWriter. Returns the number of
// docs enqueued.
func deleteDocTree(ctx context.Context, client *firestore.Client, bw *firestore.BulkWriter, ref *firestore.DocumentRef) int {
	count := 0
	cols := ref.Collections(ctx)
	for {
		col, err := cols.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Fatalf("subcollection listing error for %s: %v", ref.Path, err)
		}
		dIter := col.Documents(ctx)
		for {
			child, err := dIter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				log.Fatalf("subcollection doc iteration error under %s: %v", col.Path, err)
			}
			count += deleteDocTree(ctx, client, bw, child.Ref)
		}
		dIter.Stop()
	}
	if _, err := bw.Delete(ref); err != nil {
		log.Fatalf("failed to enqueue delete for %s: %v", ref.Path, err)
	}
	count++
	return count
}

// countDocTree counts a doc plus all of its nested subcollection docs (dry-run).
func countDocTree(ctx context.Context, ref *firestore.DocumentRef) int {
	count := 1
	cols := ref.Collections(ctx)
	for {
		col, err := cols.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Fatalf("subcollection listing error for %s: %v", ref.Path, err)
		}
		dIter := col.Documents(ctx)
		for {
			child, err := dIter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				log.Fatalf("subcollection doc iteration error under %s: %v", col.Path, err)
			}
			count += countDocTree(ctx, child.Ref)
		}
		dIter.Stop()
	}
	return count
}
