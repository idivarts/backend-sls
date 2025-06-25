package main

import (
	"context"
	"log"
	"time"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"github.com/idivarts/backend-sls/pkg/myemail"
	"google.golang.org/api/iterator"
)

func main() {
	log.Println("Syncing Users")
	syncUsers()
	log.Println("Syncing Managers")
	syncManagers()
	log.Println("Sync Completed")
}
func syncManagers() {
	iter := firestoredb.Client.Collection("managers").Documents(context.Background())
	defer iter.Stop()

	contacts := []myemail.ContactDetails{}
	for {
		doc, err := iter.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			panic(err.Error())
		}
		if time.Since(doc.UpdateTime) > 48*time.Hour {
			continue
		}

		log.Println("Creating Doc")
		manager := &trendlymodels.Manager{}
		err = doc.DataTo(manager)
		if err != nil {
			panic(err.Error())
		}
		brand, _ := trendlymodels.GetMyFirstBrand(doc.Ref.ID)
		brandName := ""
		if brand != nil {
			brandName = brand.Name
		}

		if manager.Email != "" {
			if manager.CreationTime == 0 {
				manager.CreationTime = time.Now().UnixMilli()
			}
			contacts = append(contacts, myemail.ContactDetails{
				Email:        manager.Email,
				Name:         manager.Name,
				IsManager:    true,
				CreationTime: &manager.CreationTime,
				CompanyName:  brandName,
				// Phone:             phone,
				// ProfileCompletion: pCent,
				// LastActivityTime:  manager.LastUseTime,
			})
		}
	}
	log.Println("Got all docs", len(contacts))
	for i := 0; i < len(contacts); i += 100 {
		err := myemail.CreateOrUpdateContacts(contacts[i:min(i+100, len(contacts))])
		if err != nil {
			panic(err.Error())
		}
		log.Println("Upsert Batch Complete")
	}
}

func syncUsers() {
	iter := firestoredb.Client.Collection("users").Documents(context.Background())
	defer iter.Stop()

	incompleteProfiles := 0
	contacts := []myemail.ContactDetails{}
	for {
		doc, err := iter.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			panic(err.Error())
		}
		if time.Since(doc.UpdateTime) > 48*time.Hour {
			continue
		}

		log.Println("Creating Doc")
		user := &trendlymodels.User{}
		err = doc.DataTo(user)
		if err != nil {
			panic(err.Error())
		}

		if user.Email != nil && *user.Email != "" {
			phone := ""
			pCent := 0
			if user.PhoneNumber != nil {
				phone = *user.PhoneNumber
			}
			if user.Profile != nil {
				pCent = *user.Profile.CompletionPercentage
			}
			contacts = append(contacts, myemail.ContactDetails{
				Email:             *user.Email,
				Name:              user.Name,
				Phone:             phone,
				IsManager:         false,
				ProfileCompletion: pCent,
				CreationTime:      user.CreationTime,
				LastActivityTime:  user.LastUseTime,
			})
			if pCent < 60 {
				incompleteProfiles++
			}
		}
	}
	log.Println("Got all docs", len(contacts), incompleteProfiles, ":", len(contacts)-incompleteProfiles)
	for i := 0; i < len(contacts); i += 100 {
		err := myemail.CreateOrUpdateContacts(contacts[i:min(i+100, len(contacts))])
		if err != nil {
			panic(err.Error())
		}
		log.Println("Upsert Batch Complete")
	}
}
