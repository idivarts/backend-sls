package fauth

import (
	"context"
	"log"

	"firebase.google.com/go/auth"
	firebaseapp "github.com/TrendsHub/th-backend/pkg/firebase"
)

var Client *auth.Client

func init() {
	client, err := firebaseapp.FirebaseApp.Auth(context.Background())
	if err != nil {
		log.Fatalln(err)
		panic(err.Error())
	}
	Client = client
}
