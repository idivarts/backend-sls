package fmessaging

import (
	"context"

	"firebase.google.com/go/v4/messaging"
	firebaseapp "github.com/idivarts/backend-sls/pkg/firebase"
)

var Client *messaging.Client

func init() {
	client, err := firebaseapp.FirebaseApp.Messaging(context.Background())
	if err != nil {
		panic(err)
	}
	Client = client
}
