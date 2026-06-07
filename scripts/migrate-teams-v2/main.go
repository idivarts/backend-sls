// Command migrate-teams-v2 migrates existing brands onto the team-privilege
// access model (the "Repurpose Teams inside brands" redesign). For every brand it:
//
//  1. Ensures a default team exists and grants it full access (all features +
//     all privileges).
//  2. Moves every member onto the default team (single teamId) and strips the
//     legacy member-level privilege fields (role, overrides, teamIds). Writing
//     the member back via Set() replaces the document with the new struct, so
//     legacy fields are dropped automatically.
//  3. Removes the now-unused teamId field from every brand social account.
//
// It is idempotent and safe to re-run. Pass -dry-run to log intended changes
// without writing.
//
//	go run ./scripts/migrate-teams-v2            # apply
//	go run ./scripts/migrate-teams-v2 -dry-run   # preview
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"

	_ "github.com/idivarts/backend-sls/pkg/firebase"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "log intended changes without writing")
	flag.Parse()

	ctx := context.Background()
	now := time.Now().UnixMilli()

	brandsMigrated, teamsSeeded, membersUpdated, socialsCleaned := 0, 0, 0, 0

	brandDocs, err := firestoredb.Client.Collection("brands").Documents(ctx).GetAll()
	if err != nil {
		log.Fatalf("failed to list brands: %v", err)
	}

	for _, bDoc := range brandDocs {
		brandID := bDoc.Ref.ID

		// 1. Ensure default team + grant it full access.
		defaultTeamID, err := trendlymodels.EnsureDefaultTeam(brandID, "", now)
		if err != nil {
			log.Printf("[brand %s] could not ensure default team: %v", brandID, err)
			continue
		}
		defTeam := &trendlymodels.Team{}
		if err := defTeam.Get(brandID, defaultTeamID); err != nil {
			log.Printf("[brand %s] could not load default team: %v", brandID, err)
			continue
		}
		full := trendlymodels.AllFeaturePrivilegesMap()
		if !privilegesEqual(defTeam.Privileges, full) {
			defTeam.Privileges = full
			if *dryRun {
				log.Printf("[brand %s] would seed default team %s with full privileges", brandID, defaultTeamID)
			} else if _, err := defTeam.Set(brandID); err != nil {
				log.Printf("[brand %s] failed to seed default team privileges: %v", brandID, err)
				continue
			}
			teamsSeeded++
		}

		// 2. Move every member onto the default team; drop legacy fields.
		members, err := trendlymodels.GetAllBrandMembers(brandID)
		if err != nil {
			log.Printf("[brand %s] could not list members: %v", brandID, err)
			continue
		}
		for i := range members {
			m := &members[i]
			// Re-Set unconditionally: it both assigns the default team and rewrites
			// the document without the legacy role/overrides/teamIds fields.
			m.TeamID = defaultTeamID
			if *dryRun {
				log.Printf("[brand %s] would set member %s -> team %s (drop role/overrides/teamIds)", brandID, m.ManagerID, defaultTeamID)
			} else if _, err := m.Set(brandID); err != nil {
				log.Printf("[brand %s] failed to update member %s: %v", brandID, m.ManagerID, err)
				continue
			}
			membersUpdated++
		}

		// 3. Remove teamId from brand social accounts.
		socialDocs, err := firestoredb.Client.
			Collection(fmt.Sprintf("brands/%s/socialAccounts", brandID)).
			Documents(ctx).GetAll()
		if err != nil {
			log.Printf("[brand %s] could not list socials: %v", brandID, err)
		} else {
			for _, sDoc := range socialDocs {
				if _, ok := sDoc.Data()["teamId"]; !ok {
					continue
				}
				if *dryRun {
					log.Printf("[brand %s] would remove teamId from social %s", brandID, sDoc.Ref.ID)
				} else if _, err := sDoc.Ref.Update(ctx, []firestore.Update{
					{Path: "teamId", Value: firestore.Delete},
				}); err != nil {
					log.Printf("[brand %s] failed to clean social %s: %v", brandID, sDoc.Ref.ID, err)
					continue
				}
				socialsCleaned++
			}
		}

		brandsMigrated++
	}

	mode := "applied"
	if *dryRun {
		mode = "dry-run"
	}
	log.Printf("Done (%s). brands=%d teamsSeeded=%d membersUpdated=%d socialsCleaned=%d",
		mode, brandsMigrated, teamsSeeded, membersUpdated, socialsCleaned)
}

// privilegesEqual reports whether two feature→privileges maps grant the same
// set (order-insensitive), so an already-migrated default team isn't rewritten.
func privilegesEqual(a, b map[string][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for feature, bPrivs := range b {
		aPrivs, ok := a[feature]
		if !ok || len(aPrivs) != len(bPrivs) {
			return false
		}
		set := map[string]bool{}
		for _, p := range aPrivs {
			set[p] = true
		}
		for _, p := range bPrivs {
			if !set[p] {
				return false
			}
		}
	}
	return true
}
