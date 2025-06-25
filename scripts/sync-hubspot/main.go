package main

import (
	"context"
	"log"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"github.com/idivarts/backend-sls/pkg/hubspot"
	"github.com/idivarts/backend-sls/pkg/myemail"
	"google.golang.org/api/iterator"
)

func main() {
	iter := firestoredb.Client.Collection("users").Documents(context.Background())
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
		}
	}
	log.Println("Got all docs", len(contacts))
	for i := 0; i < len(contacts); i += 100 {
		err := hubspot.CreateOrUpdateContacts(contacts[i:min(i+100, len(contacts))])
		if err != nil {
			panic(err.Error())
		}
		log.Println("Upsert Batch Complete")
	}
}
