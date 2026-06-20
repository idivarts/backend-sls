package trendlymodels

import (
	"context"
	"errors"

	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"google.golang.org/api/iterator"
)

// WebsocketConnection mirrors a websockets/{connectionId} document. One row per
// live API Gateway WebSocket connection, written on $connect, deleted on
// $disconnect (or when a send hits a stale GoneException). Used to fan messages
// out to all connections, or to the connections owned by a given user.

const websocketsCollection = "websockets"

type WebsocketConnection struct {
	Connected    bool   `json:"connected" firestore:"connected"`
	ConnectionID string `json:"connectionId" firestore:"connectionId"`
	ConnectedAt  int64  `json:"connectedAt" firestore:"connectedAt"`
	UserID       string `json:"userId,omitempty" firestore:"userId,omitempty"`
}

// SaveWebsocketConnection creates/overwrites a connection document keyed by id.
func SaveWebsocketConnection(conn *WebsocketConnection) error {
	if conn == nil || conn.ConnectionID == "" {
		return errors.New("SaveWebsocketConnection: empty connectionID")
	}
	_, err := firestoredb.Client.
		Collection(websocketsCollection).
		Doc(conn.ConnectionID).
		Set(context.Background(), conn)
	return err
}

// DeleteWebsocketConnection removes a connection document. Safe for ids that may
// not exist (used both on $disconnect and to reap stale connections).
func DeleteWebsocketConnection(connectionID string) error {
	if connectionID == "" {
		return nil
	}
	_, err := firestoredb.Client.
		Collection(websocketsCollection).
		Doc(connectionID).
		Delete(context.Background())
	return err
}

// GetWebsocketUserID returns the userId bound to a connection (set at $connect
// when a valid Firebase token was supplied). ok is false when the connection is
// unknown or unauthenticated.
func GetWebsocketUserID(connectionID string) (string, bool) {
	doc, err := firestoredb.Client.
		Collection(websocketsCollection).
		Doc(connectionID).
		Get(context.Background())
	if err != nil {
		return "", false
	}
	uid, ok := doc.Data()["userId"].(string)
	if !ok || uid == "" {
		return "", false
	}
	return uid, true
}

// AllWebsocketConnectionIDs returns every live connection id (for broadcast).
func AllWebsocketConnectionIDs() ([]string, error) {
	iter := firestoredb.Client.Collection(websocketsCollection).Documents(context.Background())
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

// WebsocketConnectionIDsByUser returns the connection ids owned by a user.
func WebsocketConnectionIDsByUser(userID string) ([]string, error) {
	if userID == "" {
		return nil, errors.New("userID is empty")
	}
	iter := firestoredb.Client.Collection(websocketsCollection).
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
