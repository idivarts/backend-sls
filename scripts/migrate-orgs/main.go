// Command migrate-orgs backfills the Organization layer for existing brands.
//
// For every onboarded, non-deleted brand that has no organizationId yet, it:
//   - creates one Organization per brand (v1 grouping rule — safest, preserves
//     each brand's billing 1:1; consolidation can be a follow-up),
//   - copies any legacy brand `billing` (read straight from the raw Firestore
//     map, since the Brand struct no longer has the field) -> Org.Billing.
//     The old credit buckets are intentionally dropped, not migrated,
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
	"encoding/json"
	"log"
	"os"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	_ "github.com/idivarts/backend-sls/pkg/firebase"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"

	"google.golang.org/api/iterator"
)

// decodeLegacyBilling pulls the legacy `billing` map (written before billing
// moved to the org) out of a raw Firestore brand document and decodes it into
// BrandBilling. Returns nil when no legacy billing is present or it cannot be
// decoded.
func decodeLegacyBilling(data map[string]interface{}) *trendlymodels.BrandBilling {
	raw, ok := data["billing"]
	if !ok || raw == nil {
		return nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	bb := &trendlymodels.BrandBilling{}
	if err := json.Unmarshal(b, bb); err != nil {
		return nil
	}
	return bb
}

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
	processed := 0

	log.Printf("fetching brands from Firestore…")
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

		processed++
		brandID := doc.Ref.ID
		log.Printf("[%d] processing brand %s", processed, brandID)

		var brand trendlymodels.Brand
		if err := doc.DataTo(&brand); err != nil {
			log.Printf("[FAIL] %s: decode brand: %v", brandID, err)
			failed++
			continue
		}

		// Idempotency: already linked to an org.
		if brand.OrganizationID != nil && *brand.OrganizationID != "" {
			log.Printf("[SKIP] %s: already linked to org %s", brandID, *brand.OrganizationID)
			skippedLinked++
			continue
		}
		if brand.DeletedAt != nil {
			log.Printf("[SKIP] %s: brand is deleted", brandID)
			skippedDeleted++
			continue
		}
		// Skip already onboarding organizations since they should have already been linked to an org;
		if brand.OrganizationID != nil && *brand.OrganizationID != "" {
			log.Printf("[SKIP] %s: onboarding complete (expected to already be linked)", brandID)
			skippedDraft++
			continue
		}

		log.Printf("    loading members for %s…", brandID)
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

		// Resolve plan + cap from the legacy raw `billing` map (the Brand struct
		// no longer has a Billing field — billing lives on the org now).
		legacyBilling := decodeLegacyBilling(doc.Data())
		planKey := "free"
		if legacyBilling != nil && legacyBilling.PlanKey != nil && *legacyBilling.PlanKey != "" {
			planKey = *legacyBilling.PlanKey
		}
		maxBrands := trendlymodels.ResolveMaxBrands(planKey)

		org := trendlymodels.Organization{
			Name:         brand.Name,
			Image:        brand.Image,
			OwnerID:      ownerID,
			BrandIds:     []string{brandID},
			Billing:      legacyBilling, // billing only — credits dropped
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

		log.Printf("    creating org for brand %s…", brandID)
		orgID, err := org.Insert()
		if err != nil {
			log.Printf("[FAIL] %s: create org: %v", brandID, err)
			failed++
			continue
		}
		log.Printf("    created org %s; seeding %d members…", orgID, len(members))

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

		log.Printf("    linking brand %s -> org %s…", brandID, orgID)
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
	log.Printf("  total processed: %d", processed)
	log.Printf("  created/would-create: %d", created)
	log.Printf("  skipped (already linked): %d", skippedLinked)
	log.Printf("  skipped (draft/not onboarded): %d", skippedDraft)
	log.Printf("  skipped (deleted): %d", skippedDeleted)
	log.Printf("  failed: %d", failed)
}
