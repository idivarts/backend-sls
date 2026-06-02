// Command fix-brand-member-status resets every brand member whose status == 0
// back to status == 1. This is a targeted repair for the regression introduced
// by the migrate-brand-roles script which inadvertently zeroed out the field.
// Safe to re-run — members already at status 1 are skipped.
package main

import (
	"context"
	"log"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"

	_ "github.com/idivarts/backend-sls/pkg/firebase"
)

func main() {
	ctx := context.Background()

	brandsProcessed, membersFixed := 0, 0

	brandDocs, err := firestoredb.Client.Collection("brands").Documents(ctx).GetAll()
	if err != nil {
		log.Fatalf("failed to list brands: %v", err)
	}

	for _, bDoc := range brandDocs {
		brandID := bDoc.Ref.ID

		members, err := trendlymodels.GetAllBrandMembers(brandID)
		if err != nil {
			log.Printf("[brand %s] could not list members: %v", brandID, err)
			continue
		}

		for _, m := range members {
			if m.Status != 0 {
				continue
			}

			_, err := firestoredb.Client.
				Collection("brands").Doc(brandID).
				Collection("members").Doc(m.ManagerID).
				Update(ctx, []firestore.Update{{Path: "status", Value: 1}})
			if err != nil {
				log.Printf("[brand %s] failed to fix member %s: %v", brandID, m.ManagerID, err)
				continue
			}

			log.Printf("[brand %s] fixed member %s status 0 → 1", brandID, m.ManagerID)
			membersFixed++
		}

		brandsProcessed++
	}

	log.Printf("Done. brandsProcessed=%d membersFixed=%d", brandsProcessed, membersFixed)
}
