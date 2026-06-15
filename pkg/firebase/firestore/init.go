package firestoredb

import (
	"context"
	"log"
	"os"

	"cloud.google.com/go/firestore"
	firebaseapp "github.com/idivarts/backend-sls/pkg/firebase"
	"google.golang.org/api/option"
)

var Client *firestore.Client

func init() {
	ctx := context.Background()

	dbID := os.Getenv("FIRESTORE_DATABASE_ID")
	if dbID == "" {
		dbID = "(default)"
	}
	log.Printf("Creating Firestore (project=%s db=%s)", firebaseapp.ProjectID, dbID)

	var err error
	Client, err = firestore.NewClientWithDatabase(ctx, firebaseapp.ProjectID, dbID, option.WithCredentialsFile(firebaseapp.ConfigFile))
	if err != nil {
		log.Fatalln(err)
		panic(err.Error())
	}
	log.Println("Created Firestore Connection")
}
