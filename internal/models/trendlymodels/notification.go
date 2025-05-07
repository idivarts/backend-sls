package trendlymodels

import (
	"context"
	"errors"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

type NotificationData struct {
	CollaborationID *string `json:"collaborationId,omitempty" firestore:"collaborationId,omitempty"`
	GroupID         *string `json:"groupId,omitempty" firestore:"groupId,omitempty"`
	UserID          *string `json:"userId,omitempty" firestore:"userId,omitempty"`
}

type Notification struct {
	Title       string            `json:"title" firestore:"title"`
	Description string            `json:"description" firestore:"description"`
	TimeStamp   int64             `json:"timeStamp" firestore:"timeStamp"`
	IsRead      bool              `json:"isRead" firestore:"isRead"`
	Data        *NotificationData `json:"data,omitempty" firestore:"data,omitempty"`
	Type        string            `json:"type" firestore:"type"`
}

type NotificationCollection string

const (
	USER_COLLECTION    NotificationCollection = "users"
	MANAGER_COLLECTION NotificationCollection = "managers"
	BRAND_COLLECTION   NotificationCollection = "brands"
)

var (
	errorCollection = errors.New("invalid-collection-passed")
)

func (u *Notification) Insert(collection NotificationCollection, id string) (*firestore.DocumentRef, *firestore.WriteResult, error) {
	if collection == USER_COLLECTION {
		return sendUnitNotification(string(collection), id, u)
	} else if collection == MANAGER_COLLECTION {
		return sendUnitNotification(string(collection), id, u)
	} else if collection == BRAND_COLLECTION {
		bMembers, err := GetAllBrandMembers(id)
		if err != nil {
			return nil, nil, err
		}
		var x *firestore.DocumentRef
		var y *firestore.WriteResult
		for _, v := range bMembers {
			// Just sending the last insert details for the sake of keeping it consistent
			x, y, _ = sendUnitNotification(string(MANAGER_COLLECTION), v.ManagerID, u)
		}
		return x, y, nil
	}
	return nil, nil, errorCollection
}

func sendUnitNotification(collection, id string, u *Notification) (*firestore.DocumentRef, *firestore.WriteResult, error) {
	res, wRes, err := firestoredb.Client.Collection(collection).Doc(id).Collection("notifications").Add(context.Background(), u)

	if err != nil {
		return nil, nil, err
	}
	return res, wRes, err
}
