// Command backfill-org-brands walks every non-deleted brand and:
//
//   - finalizes onboarding on every brand (onboardingComplete=true), so legacy
//     drafts surface in the brand switcher,
//   - for org-linked brands, repairs membership left broken by the early
//     AddBrandToOrganization path (which didn't create a BrandMember, so the
//     brand never showed up in the creator's switcher): ensures every org
//     member (and the org owner) has an ACTIVE BrandMember on the brand;
//     existing memberships are never clobbered,
//   - ensures a default team exists for org-linked brands.
//
// Idempotent. Dry-run by default; set APPLY=1 to write.
//
//	go run ./scripts/backfill-org-brands          # dry-run (no writes)
//	APPLY=1 go run ./scripts/backfill-org-brands  # apply
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
	apply := os.Getenv("APPLY") == "1"
	mode := "DRY-RUN (no writes) — set APPLY=1 to apply"
	if apply {
		mode = "APPLY (writing changes)"
	}
	log.Printf("backfill-org-brands starting — %s", mode)

	ctx := context.Background()
	now := time.Now().UnixMilli()
	var (
		processed, skippedDeleted, skippedNoOrgDoc, failed int
		onboardingSet, membersAdded                        int
	)

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

		if brand.DeletedAt != nil {
			skippedDeleted++
			continue
		}

		needsOnboarding := !brand.OnboardingComplete

		var (
			hasOrg  = brand.OrganizationID != nil && *brand.OrganizationID != ""
			orgID   string
			org     trendlymodels.Organization
			missing []string
		)
		if hasOrg {
			orgID = *brand.OrganizationID
			if err := org.Get(orgID); err != nil {
				log.Printf("[WARN] %s: org %s not found (skipping org repair, still finalizing onboarding): %v", brandID, orgID, err)
				skippedNoOrgDoc++
				hasOrg = false
			}
		}
		if hasOrg {
			// Collect the managers who should have access to this brand: the
			// org owner + every org member.
			wantMembers := map[string]bool{}
			if org.OwnerID != "" {
				wantMembers[org.OwnerID] = true
			}
			omIter := firestoredb.Client.Collection("organizations").Doc(orgID).
				Collection("orgMembers").Documents(ctx)
			for {
				omDoc, err := omIter.Next()
				if err != nil {
					if err == iterator.Done {
						break
					}
					log.Printf("[WARN] %s: list orgMembers of %s: %v", brandID, orgID, err)
					break
				}
				var om trendlymodels.OrganizationMember
				if err := omDoc.DataTo(&om); err != nil {
					continue
				}
				if om.ManagerID != "" {
					wantMembers[om.ManagerID] = true
				}
			}
			omIter.Stop()

			// Find which of those are missing a BrandMember (don't clobber
			// existing ones — their team/status stay as-is).
			for managerID := range wantMembers {
				bm := &trendlymodels.BrandMember{}
				if err := bm.Get(brandID, managerID); err != nil {
					missing = append(missing, managerID)
				}
			}
		}

		if !needsOnboarding && len(missing) == 0 {
			processed++
			continue // already healthy
		}

		if !apply {
			log.Printf("[DRY] brand %q (%s) hasOrg=%v → setOnboarding=%v, add %d member(s) %v",
				brand.Name, brandID, hasOrg, needsOnboarding, len(missing), missing)
			processed++
			if needsOnboarding {
				onboardingSet++
			}
			membersAdded += len(missing)
			continue
		}

		if hasOrg && len(missing) > 0 {
			// Ensure a default team exists (created by the org owner when possible).
			teamCreator := org.OwnerID
			if teamCreator == "" {
				teamCreator = missing[0]
			}
			defTeam, err := trendlymodels.EnsureDefaultTeam(brandID, teamCreator, now)
			if err != nil {
				log.Printf("[FAIL] %s: ensure default team: %v", brandID, err)
				failed++
				continue
			}

			for _, managerID := range missing {
				m := trendlymodels.BrandMember{ManagerID: managerID, Status: 1, TeamID: defTeam}
				if _, err := m.Set(brandID); err != nil {
					log.Printf("[WARN] %s: add member %s: %v", brandID, managerID, err)
					continue
				}
				membersAdded++
			}
		}

		brandUpdates := []firestore.Update{}
		if needsOnboarding {
			brandUpdates = append(brandUpdates, firestore.Update{Path: "onboardingComplete", Value: true})
		}
		if len(brandUpdates) > 0 {
			if _, err := firestoredb.Client.Collection("brands").Doc(brandID).Update(ctx, brandUpdates); err != nil {
				log.Printf("[WARN] %s: update brand fields: %v", brandID, err)
			} else if needsOnboarding {
				onboardingSet++
			}
		}

		log.Printf("[OK] brand %q (%s) hasOrg=%v → setOnboarding=%v, +%d member(s)",
			brand.Name, brandID, hasOrg, needsOnboarding, len(missing))
		processed++
	}

	log.Printf("backfill-org-brands done — %s", mode)
	log.Printf("  brands processed:            %d", processed)
	log.Printf("  onboardingComplete set:      %d", onboardingSet)
	log.Printf("  brand memberships added:     %d", membersAdded)
	log.Printf("  skipped (deleted):           %d", skippedDeleted)
	log.Printf("  skipped (org doc missing):   %d", skippedNoOrgDoc)
	log.Printf("  failed:                      %d", failed)
}
