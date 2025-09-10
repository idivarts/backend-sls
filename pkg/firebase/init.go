package firebaseapp

import (
	"context"
	"log"
	"os"

	// firebase "firebase.google.com/go"
	firebase "firebase.google.com/go/v4"
	"google.golang.org/api/option"
)

var FirebaseApp *firebase.App

func init() {
	// Use a service account
	ctx := context.Background()
	configFile := os.Getenv("FIREBASE_CONFIG_PATH")
	log.Println("Config File Path", configFile)
	if configFile == "" {
		configFile = "/Users/rsinha/iDiv/backend-sls/service-account.json"
	}
	sa := option.WithCredentialsFile(configFile)
	log.Println("Coming here", sa)
	var err error
	FirebaseApp, err = firebase.NewApp(ctx, nil, sa)
	if err != nil {
		log.Fatalln(err)
		panic(err.Error())
	}
	log.Println("Success Connection")
}
