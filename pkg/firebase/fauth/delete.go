package fauth

import (
	"context"
	"fmt"
	"log"

	"google.golang.org/api/iterator"
)

func DeleteAllUsers() {
	// Client.GetUsers(context.Background(), []auth.UserIdentifier{})
	// Use the ListUsers method to paginate through the users
	uids := []string{}
	iter := Client.Users(context.Background(), "")
	for {
		userRecord, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Fatalf("Error listing users: %v", err)
		}

		// Print user details
		fmt.Printf("User UID: %s, Email: %s\n", userRecord.UID, userRecord.Email)
		uids = append(uids, userRecord.UID)
	}
	log.Println("Deleting the users", len(uids))

	for i := range uids {
		Client.DeleteUser(context.Background(), uids[i])
		log.Println("Deleted user at", i, uids[i])
	}
}
