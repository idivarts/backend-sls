package main

import (
	"context"
	"encoding/csv"
	"log"
	"os"
	"strings"

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

	file, err := os.Create("new-leads.csv")
	if err != nil {
		log.Fatalf("Failed to create CSV file: %v", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write CSV header
	writer.Write([]string{"Email", "First Name", "Last Name", "Phone"})

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

		myNames := strings.Split(manager.Name, " ")

		log.Println(doc.Ref.ID, manager.Name, ":", manager.Email, "  -> ", brand.Name, " - ", myutil.DerefString(brand.Profile.PhoneNumber))
		writer.Write([]string{
			manager.Email,
			myNames[0],
			strings.Join(myNames[1:], " "),
			myutil.DerefString(brand.Profile.PhoneNumber),
		})
	}
	log.Println("All New Leads Exported")
}
