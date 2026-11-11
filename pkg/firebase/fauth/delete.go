package fauth

import (
	"context"
	"fmt"
	"log"

	"google.golang.org/api/iterator"
)

// DeleteAllUsers deletes EVERY Firebase Auth user in the project.
//
// ☠️ THIS IS NOT RECOVERABLE AND IT IS NOT SCOPED TO AN ENVIRONMENT.
// Firebase Auth is per PROJECT, not per Firestore database, so "dev" and
// "prod" share one user pool: running this once removes every login for
// both. The profile documents in `managers` and `users` survive, which makes
// it worse rather than better — the data is still there and nobody can reach
// it, because an account recreated by signing up again gets a NEW uid and no
// longer matches the documents keyed by the old one.
//
// ⚠️ IT USED TO HAVE A TEST. `pkg/firebase/fauth/delete_test.go` was four
// lines — `func TestDelete(t *testing.T) { fauth.DeleteAllUsers() }` — with no
// build tag and no guard, against the live service account. So `go test ./...`
// wiped the project's entire user pool, and `go test ./...` is the ordinary
// gate anyone runs before opening a PR. On 2026-11-12 it did exactly that:
// every auth user in trendly-9ab99 was deleted by a routine pre-PR test run.
//
// The test is gone. If you need this function, call it from a deliberate
// one-off with the project printed and a confirmation read from stdin. Do not
// give it a test, and do not give it a `TestMain` — the whole hazard is that
// it looks like test code.
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
