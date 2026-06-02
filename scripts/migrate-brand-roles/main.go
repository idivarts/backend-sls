// Command migrate-brand-roles backfills the privilege-control redesign onto
// existing brands. For every brand it:
//
//  1. Ensures a default team exists.
//  2. Remaps legacy member roles to the new RBAC roles:
//     - legacy "manager" (the brand creator) -> Owner
//     - everyone else                        -> Admin
//     Members that already carry a valid new role are left untouched
//     (idempotent — safe to re-run).
//  3. Guarantees at least one Owner per brand.
//  4. Scopes every member and every connected brand social to the default team
//     when they don't already belong to one.
package main

import (
	"context"
	"log"
	"time"

	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"

	_ "github.com/idivarts/backend-sls/pkg/firebase"
)

func main() {
	ctx := context.Background()
	now := time.Now().UnixMilli()

	brandsMigrated, membersUpdated, socialsUpdated := 0, 0, 0

	brandDocs, err := firestoredb.Client.Collection("brands").Documents(ctx).GetAll()
	if err != nil {
		log.Fatalf("failed to list brands: %v", err)
	}

	for _, bDoc := range brandDocs {
		brandID := bDoc.Ref.ID

		defaultTeamID, err := trendlymodels.EnsureDefaultTeam(brandID, "", now)
		if err != nil {
			log.Printf("[brand %s] could not ensure default team: %v", brandID, err)
			continue
		}

		members, err := trendlymodels.GetAllBrandMembers(brandID)
		if err != nil {
			log.Printf("[brand %s] could not list members: %v", brandID, err)
			continue
		}

		ownerCount := 0
		for i := range members {
			m := &members[i]
			changed := false

			// Remap legacy roles only; preserve already-migrated roles.
			if !m.Role.IsValid() {
				if string(m.Role) == "manager" {
					m.Role = trendlymodels.RoleOwner
				} else {
					m.Role = trendlymodels.RoleAdmin
				}
				changed = true
			}
			if m.Role == trendlymodels.RoleOwner {
				ownerCount++
			}

			// Scope to the default team if not already in a team.
			if len(m.TeamIDs) == 0 {
				m.TeamIDs = []string{defaultTeamID}
				changed = true
			}

			if changed {
				if _, err := m.Set(brandID); err != nil {
					log.Printf("[brand %s] failed to update member %s: %v", brandID, m.ManagerID, err)
					continue
				}
				membersUpdated++
			}
		}

		// Guarantee at least one Owner.
		if ownerCount == 0 && len(members) > 0 {
			promote := &members[0]
			promote.Role = trendlymodels.RoleOwner
			if _, err := promote.Set(brandID); err != nil {
				log.Printf("[brand %s] failed to promote owner %s: %v", brandID, promote.ManagerID, err)
			} else {
				membersUpdated++
				log.Printf("[brand %s] no owner found — promoted %s to Owner", brandID, promote.ManagerID)
			}
		}

		// Scope existing brand socials to the default team.
		socials, err := trendlymodels.ListBrandSocialAccounts(brandID)
		if err != nil {
			log.Printf("[brand %s] could not list socials: %v", brandID, err)
		} else {
			for _, s := range socials {
				if s.TeamID == "" {
					if _, err := trendlymodels.AssignBrandSocialTeam(brandID, s.ID, defaultTeamID, now); err != nil {
						log.Printf("[brand %s] failed to assign social %s to team: %v", brandID, s.ID, err)
						continue
					}
					socialsUpdated++
				}
			}
		}

		brandsMigrated++
	}

	log.Printf("Done. brands=%d membersUpdated=%d socialsUpdated=%d", brandsMigrated, membersUpdated, socialsUpdated)
}
