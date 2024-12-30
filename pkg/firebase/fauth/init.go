package fauth

import (
	"context"
	"log"

	"firebase.google.com/go/v4/auth"
	firebaseapp "github.com/idivarts/backend-sls/pkg/firebase"
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
