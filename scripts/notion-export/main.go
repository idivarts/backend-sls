package main

import (
	"context"
	"encoding/csv"
	"log"
	"os"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"github.com/idivarts/backend-sls/pkg/myutil"
	"google.golang.org/api/iterator"
)

const (
	DAYS_BACK = 2
)

func main() {
	iter := firestoredb.Client.Collection("brands").Documents(context.Background())
	defer iter.Stop() // Always defer stopping the iterator

	file, err := os.Create("new-notion-leads.csv")
	if err != nil {
		log.Fatalf("Failed to create CSV file: %v", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write CSV header
	writer.Write([]string{"Name", "Company", "Email", "Phone", "Industry", "Website", "Lead Source"})

	for {
		doc, err := iter.Next()
		if err != nil {
			if err == iterator.Done {
				break // No more documents
			}
			log.Fatalf("Error iterating documents: %v", err)
		}

		// if time.Since(doc.CreateTime) < 48*time.Hour {
		// 	continue
		// }

		brand := trendlymodels.Brand{}
		err = doc.DataTo(&brand)
		if err != nil {
			log.Println("Error :", err.Error())
			continue
		}
		if brand.Profile == nil {
			continue
		}

		members, err := trendlymodels.GetAllBrandMembers(doc.Ref.ID)
		if err != nil {
			log.Println("Error :", err.Error())
			continue
		}
		if len(members) == 0 {
			log.Println("No Members Found")
			continue
		}
		firstMember := members[0]

		manager := trendlymodels.Manager{}
		err = manager.Get(firstMember.ManagerID)
		if err != nil {
			log.Println("Error :", err.Error())
			continue
		}

		phone := ""
		if brand.Profile != nil && brand.Profile.PhoneNumber != nil {
			phone = myutil.DerefString(brand.Profile.PhoneNumber)
		}

		industry := ""
		if brand.Profile != nil && brand.Profile.Industries != nil && len(brand.Profile.Industries) > 0 {
			industry = myutil.DerefString(&brand.Profile.Industries[0])
		}

		website := ""
		if brand.Profile != nil && brand.Profile.Website != nil {
			website = myutil.DerefString(brand.Profile.Website)
		}

		log.Println(doc.Ref.ID, manager.Name, ":", manager.Email, "  -> ", brand.Name, " - ", phone)
		// "Name", "Company", "Email", "Phone", "Industry", "Website", "Lead Source"
		writer.Write([]string{
			manager.Name,
			brand.Name,
			manager.Email,
			phone,
			industry,
			website,
			"Trendly",
		})
	}
	log.Println("All New Leads Exported")
}
