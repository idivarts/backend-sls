package main

import (
	"context"
	"log"

	"cloud.google.com/go/firestore"
	"github.com/idivarts/backend-sls/pkg/firebase/fauth"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

func main() {

	iter := firestoredb.Client.Collection("users").Documents(context.Background())
	defer iter.Stop()
	for {
		doc, err := iter.Next()
		if err != nil {
			if err.Error() == "iterator: done" {
				break
			}
			log.Fatalf("Error getting document: %v", err)
			return
		}
		if doc.Data()["creationTime"] == nil {
			// log.Printf("Document ID: %s, Data: %v", doc.Ref.ID, doc.Data())
			user, err := fauth.Client.GetUser(context.Background(), doc.Ref.ID)
			if err != nil {
				log.Fatalf("Error getting documents: %v", err)
				continue
			}
			ts := user.UserMetadata.CreationTimestamp
			// log.Printf("User ID: %s, Creation Timestamp: %d", doc.Ref.ID, ts)

			_, err = firestoredb.Client.Collection("users").Doc(doc.Ref.ID).Set(context.Background(), map[string]interface{}{
				"creationTime": ts,
			}, firestore.MergeAll)
			if err != nil {
				log.Fatalf("Error setting document: %v", err)
				continue
			}

			log.Printf("Document ID: %s, Creation Time: %d", doc.Ref.ID, ts)
			// return
		}
	}
}
