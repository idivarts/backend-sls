package main

import (
	"context"
	"log"
	"time"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"github.com/idivarts/backend-sls/pkg/myquery"
	"google.golang.org/api/iterator"
)

func main() {
	str := myquery.Client.Project()
	log.Println("Client ProjectID", str)

	query := myquery.Client.Query("SELECT * FROM `trendly-9ab99.matches.influencers` LIMIT 1000")
	_, err := query.Read(context.Background())
	if err != nil {
		log.Fatalf("Failed to execute query: %v", err)
	}
	log.Println("Successful Connection")

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
