package main

import (
	"context"
	"log"

	"firebase.google.com/go/v4/auth"
	"github.com/idivarts/backend-sls/pkg/firebase/fauth"
)

const (
	USER_EMAIL  = "serialchillerglobal@gmail.com"
	newPassword = "Trendly@123" // Set the new password here
)

func main() {
	user, err := fauth.Client.GetUserByEmail(context.Background(), USER_EMAIL)
	if err != nil {
		panic(err)
	}
	params := (&auth.UserToUpdate{}).Password(newPassword)
	_, err = fauth.Client.UpdateUser(context.Background(), user.UID, params)
	if err != nil {
		log.Fatalf("error updating user password: %v\n", err)
	}

	log.Printf("Password updated for user: %s\n", user.Email)
}
