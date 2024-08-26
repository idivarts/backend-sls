package firestoredb_test

import (
	"context"
	"fmt"
	"log"
	"testing"

	firestoredb "github.com/TrendsHub/th-backend/pkg/firebase/firestore"
	"google.golang.org/api/iterator"
)

func TestFireStoreConnection(t *testing.T) {
	iter := firestoredb.Client.Collection("users").Documents(context.Background())
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Fatalf("Failed to iterate: %v", err)
		}
		fmt.Println(doc.Data())
	}
}
