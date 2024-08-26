package wshandler

import (
	"context"
	"log"

	"google.golang.org/api/iterator"
)

func Broadcast(data string) error {
	iter := firestoreClient.Collection("websockets").Documents(context.Background())
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Fatalf("Failed to iterate: %v", err)
		}
		SendToConnection(&doc.Ref.ID, data)
		// fmt.Println(doc.Data())
	}
	// if err != nil {
	// 	log.Printf("Failed to scan connections: %v", err)
	// 	return err
	// }

	// for _, item := range connections.Items {
	// 	connectionID := item["connectionId"].S
	// 	SendToConnection(connectionID, data)
	// }
	return nil
}
