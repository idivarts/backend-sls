package trendlymodels

import (
	"context"
	"errors"

	"firebase.google.com/go/v4/messaging"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"github.com/idivarts/backend-sls/pkg/firebase/fmessaging"
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
	errorCollection      = errors.New("invalid-collection-passed")
	errorCollectionFetch = errors.New("user-manager-collection-fetch-error")
)

func (u *Notification) Insert(collection NotificationCollection, id string) (*messaging.BatchResponse, []string, error) {
	tokens := []string{}
	emails := []string{}
	if collection == USER_COLLECTION {
		t, e, err := sendUnitNotification(collection, id, u)
		if err != nil {
			return nil, nil, err
		}
		tokens = append(tokens, t...)
		emails = append(emails, *e)
	} else if collection == MANAGER_COLLECTION {
		t, e, err := sendUnitNotification(collection, id, u)
		if err != nil {
			return nil, nil, err
		}
		tokens = append(tokens, t...)
		emails = append(emails, *e)
	} else if collection == BRAND_COLLECTION {
		bMembers, err := GetAllBrandMembers(id)
		if err != nil {
			return nil, nil, err
		}
		for _, v := range bMembers {
			// Just sending the last insert details for the sake of keeping it consistent
			t, e, err := sendUnitNotification(MANAGER_COLLECTION, v.ManagerID, u)
			if err != nil {
				return nil, nil, err
			}
			tokens = append(tokens, t...)
			emails = append(emails, *e)
		}
	} else {
		return nil, nil, errorCollection
	}

	if len(tokens) > 0 {
		r, e := fmessaging.Client.SendEachForMulticast(context.Background(), &messaging.MulticastMessage{
			Tokens: tokens,
			Data:   map[string]string{},
			Notification: &messaging.Notification{
				Title: u.Title,
				Body:  u.Description,
			},
			Android: &messaging.AndroidConfig{
				Priority: "high",
				Notification: &messaging.AndroidNotification{
					Sound: "",
				},
			},
			Webpush: &messaging.WebpushConfig{
				Notification: &messaging.WebpushNotification{
					Silent: false,
				},
			},
			APNS: &messaging.APNSConfig{
				Payload: &messaging.APNSPayload{
					Aps: &messaging.Aps{
						Sound: "",
					},
				},
			},
			FCMOptions: &messaging.FCMOptions{},
		})
		return r, nil, e
	}
	return nil, nil, nil
}

func sendUnitNotification(collection NotificationCollection, id string, u *Notification) ([]string, *string, error) {
	tokens := []string{}
	emails := ""
	if collection == USER_COLLECTION {
		user := &User{}
		err := user.Get(id)
		if err != nil {
			return nil, nil, errorCollectionFetch
		}
		tokens = append(tokens, user.PushNotificationToken.Web...)
		tokens = append(tokens, user.PushNotificationToken.IOS...)
		tokens = append(tokens, user.PushNotificationToken.Android...)
		emails = *user.Email
	} else if collection == MANAGER_COLLECTION {
		manager := &Manager{}
		err := manager.Get(id)
		if err != nil {
			return nil, nil, errorCollectionFetch
		}
		tokens = append(tokens, manager.PushNotificationToken.Web...)
		tokens = append(tokens, manager.PushNotificationToken.IOS...)
		tokens = append(tokens, manager.PushNotificationToken.Android...)
		emails = manager.Email
	}
	_, _, err := firestoredb.Client.Collection(string(collection)).Doc(id).Collection("notifications").Add(context.Background(), u)
	if err != nil {
		return nil, nil, err
	}
	return tokens, &emails, err
}
