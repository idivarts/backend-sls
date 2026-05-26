package wshandler

import (
	"context"
	"errors"
	"log"

	"google.golang.org/api/iterator"
)

func GetUserConnections(userID string) ([]string, error) {
	if userID == "" {
		return nil, errors.New("userID is empty")
	}
	iter := firestoreClient.Collection("websockets").
		Where("userId", "==", userID).
		Documents(context.Background())
	defer iter.Stop()

	var ids []string
	for {
		doc, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		ids = append(ids, doc.Ref.ID)
	}
	return ids, nil
}

func SendToUser(userID string, data string) {
	conns, err := GetUserConnections(userID)
	if err != nil {
		log.Printf("ws SendToUser: lookup failed for %s: %v", userID, err)
		return
	}
	for i := range conns {
		id := conns[i]
		SendToConnection(&id, data)
	}
}

func SendToConnections(conns []string, data string) {
	for i := range conns {
		id := conns[i]
		SendToConnection(&id, data)
	}
}
