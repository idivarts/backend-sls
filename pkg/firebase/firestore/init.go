package firestoredb

import (
	"context"
	"log"

	"cloud.google.com/go/firestore"
	firebaseapp "github.com/idivarts/backend-sls/pkg/firebase"
)

var Client *firestore.Client

func init() {
	ctx := context.Background()
	var err error
	log.Println("Creating Firestore")

	Client, err = firebaseapp.FirebaseApp.Firestore(ctx)
	if err != nil {
		log.Fatalln(err)
		panic(err.Error())
	}
	log.Println("Created Firestore Connection")
}
