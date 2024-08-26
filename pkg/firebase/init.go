package firebaseapp

import (
	"context"
	"log"

	// firebase "firebase.google.com/go"
	firebase "firebase.google.com/go"
	"google.golang.org/api/option"
)

var FirebaseApp *firebase.App

func init() {
	// Use a service account
	ctx := context.Background()
	sa := option.WithCredentialsFile("service-account.json")
	var err error
	FirebaseApp, err = firebase.NewApp(ctx, nil, sa)
	if err != nil {
		log.Fatalln(err)
		panic(err.Error())
	}
}
