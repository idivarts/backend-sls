package main

import (
	"context"
	"log"
	"time"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"google.golang.org/api/iterator"
)

func main() {
	log.Println("Syncing Users")
	syncUsers(true)
	log.Println("Syncing Brands")
	syncBrands(false)
	log.Println("Sync Completed")
}
func syncBrands(iterative bool) {
	iter := firestoredb.Client.Collection("brands").Documents(context.Background())
	defer iter.Stop()

	for {
		doc, err := iter.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			panic(err.Error())
		}
		if iterative && time.Since(doc.UpdateTime) > 28*time.Hour {
			continue
		}

		log.Println("Creating Doc")
		manager := &trendlymodels.Brand{}
		err = doc.DataTo(manager)
		if err != nil {
			panic(err.Error())
		}
	}

}

func syncUsers(iterative bool) {
	iter := firestoredb.Client.Collection("users").Documents(context.Background())
	defer iter.Stop()

	for {
		doc, err := iter.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			panic(err.Error())
		}
		if iterative && time.Since(doc.UpdateTime) > 28*time.Hour {
			continue
		}

		log.Println("Creating Doc")
		user := &trendlymodels.User{}
		err = doc.DataTo(user)
		if err != nil {
			panic(err.Error())
		}

	}

}
