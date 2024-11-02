package firestoredb_test

import (
	"context"
	"fmt"
	"log"
	"testing"

	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
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

func TestAddAgainstRule(t *testing.T) {
	_, _, err := firestoredb.Client.Collection("userstest").Add(context.Background(), map[string]interface{}{
		"test": "Hello there",
	})
	if err != nil {
		log.Fatalf("Failed adding alovelace: %v", err)
	}
}
