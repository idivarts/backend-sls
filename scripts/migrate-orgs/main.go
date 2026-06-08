// Command migrate-orgs backfills the Organization layer for existing brands.
//
// For every onboarded, non-deleted brand that has no organizationId yet, it:
//   - creates one Organization per brand (v1 grouping rule — safest, preserves
//     each brand's billing 1:1; consolidation can be a follow-up),
//   - copies Brand.Billing -> Org.Billing (BILLING ONLY; the old credit buckets
//     are intentionally dropped, not migrated),
//   - resolves maxBrands from the plan,
//   - seeds org membership from the brand's members (owner -> org_owner, rest
//     -> member),
//   - stamps brand.organizationId so the brand is linked and the run is
//     idempotent (a re-run skips already-linked brands).
//
// Dry-run by default. Set MIGRATE_APPLY=1 to actually write.
//
//	go run ./scripts/migrate-orgs            # dry-run (no writes)
//	MIGRATE_APPLY=1 go run ./scripts/migrate-orgs   # apply
package main

import (
	"context"
	"log"
	"os"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	_ "github.com/idivarts/backend-sls/pkg/firebase"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"

	"google.golang.org/api/iterator"
)

func main() {
	apply := os.Getenv("MIGRATE_APPLY") == "1"
	mode := "DRY-RUN (no writes) — set MIGRATE_APPLY=1 to apply"
	if apply {
		mode = "APPLY (writing changes)"
	}
	log.Printf("migrate-orgs starting — %s", mode)

	ctx := context.Background()
	now := time.Now().UnixMilli()
	created, skippedLinked, skippedDraft, skippedDeleted, failed := 0, 0, 0, 0, 0

	iter := firestoredb.Client.Collection("brands").Documents(ctx)
	defer iter.Stop()

	for {
		doc, err := iter.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			log.Fatalf("brand iteration error: %v", err)
		}

		brandID := doc.Ref.ID
		var brand trendlymodels.Brand
		if err := doc.DataTo(&brand); err != nil {
			log.Printf("[FAIL] %s: decode brand: %v", brandID, err)
			failed++
			continue
		}

		// Idempotency: already linked to an org.
		if brand.OrganizationID != nil && *brand.OrganizationID != "" {
			skippedLinked++
			continue
		}
		if brand.DeletedAt != nil {
			skippedDeleted++
			continue
		}
		// Skip abandoned draft brands (onboarding never finished).
		if !brand.OnboardingComplete {
			skippedDraft++
			continue
		}

		// Resolve owner from brand members (prefer an active member).
		members, err := trendlymodels.GetAllBrandMembers(brandID)
		if err != nil {
			log.Printf("[FAIL] %s: load members: %v", brandID, err)
			failed++
			continue
		}
		ownerID := ""
		for _, m := range members {
			if m.Status == 1 {
				ownerID = m.ManagerID
				break
			}
		}
		if ownerID == "" && len(members) > 0 {
			ownerID = members[0].ManagerID
		}

		// Resolve plan + cap. Default to free when no plan is set.
		planKey := "free"
		if brand.Billing != nil && brand.Billing.PlanKey != nil && *brand.Billing.PlanKey != "" {
			planKey = *brand.Billing.PlanKey
		}
		maxBrands := trendlymodels.ResolveMaxBrands(planKey)

		org := trendlymodels.Organization{
			Name:         brand.Name,
			Image:        brand.Image,
			OwnerID:      ownerID,
			BrandIds:     []string{brandID},
			Billing:      brand.Billing, // billing only — credits dropped
			PlanKey:      &planKey,
			MaxBrands:    maxBrands,
			CreationTime: now,
		}

		if !apply {
			log.Printf("[DRY] brand %q (%s) -> new org name=%q owner=%s plan=%s maxBrands=%d members=%d",
				brand.Name, brandID, org.Name, ownerID, planKey, maxBrands, len(members))
			created++
			continue
		}

		orgID, err := org.Insert()
		if err != nil {
			log.Printf("[FAIL] %s: create org: %v", brandID, err)
			failed++
			continue
		}

		// Seed org membership from brand members.
		for _, m := range members {
			role := trendlymodels.OrgRoleMember
			if m.ManagerID == ownerID {
				role = trendlymodels.OrgRoleOwner
			}
			om := trendlymodels.OrganizationMember{ManagerID: m.ManagerID, Role: role, Status: 1}
			if _, err := om.Set(orgID); err != nil {
				log.Printf("[WARN] %s: seed org member %s: %v", brandID, m.ManagerID, err)
			}
		}

		// Link the brand to its new org.
		if _, err := firestoredb.Client.Collection("brands").Doc(brandID).
			Update(ctx, []firestore.Update{{Path: "organizationId", Value: orgID}}); err != nil {
			log.Printf("[FAIL] %s: link brand to org %s: %v", brandID, orgID, err)
			failed++
			continue
		}

		log.Printf("[OK] brand %q (%s) -> org %s (owner=%s plan=%s maxBrands=%d)",
			brand.Name, brandID, orgID, ownerID, planKey, maxBrands)
		created++
	}

	log.Printf("migrate-orgs done — %s", mode)
	log.Printf("  created/would-create: %d", created)
	log.Printf("  skipped (already linked): %d", skippedLinked)
	log.Printf("  skipped (draft/not onboarded): %d", skippedDraft)
	log.Printf("  skipped (deleted): %d", skippedDeleted)
	log.Printf("  failed: %d", failed)
}
