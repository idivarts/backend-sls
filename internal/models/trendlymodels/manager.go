package trendlymodels

import (
	"context"

	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

type ManagerSettings struct {
	Theme             string `json:"theme,omitempty" firestore:"theme,omitempty"` // "light" or "dark"
	EmailNotification bool   `json:"emailNotification,omitempty" firestore:"emailNotification,omitempty"`
	PushNotification  bool   `json:"pushNotification,omitempty" firestore:"pushNotification,omitempty"`
}

type Manager struct {
	Name                  string                `json:"name" firestore:"name"`
	Email                 string                `json:"email" firestore:"email"`
	IsAdmin               bool                  `json:"isAdmin" firestore:"isAdmin"`
	PhoneNumber           string                `json:"phoneNumber,omitempty" firestore:"phoneNumber,omitempty"`
	Location              string                `json:"location,omitempty" firestore:"location,omitempty"`
	IsChatConnected       bool                  `json:"isChatConnected,omitempty" firestore:"isChatConnected,omitempty"`
	ProfileImage          string                `json:"profileImage,omitempty" firestore:"profileImage,omitempty"`
	Settings              *ManagerSettings      `json:"settings,omitempty" firestore:"settings,omitempty"`
	PushNotificationToken PushNotificationToken `json:"pushNotificationToken" firestore:"pushNotificationToken"`
}

func (u *Manager) Get(managerId string) error {
	res, err := firestoredb.Client.Collection("managers").Doc(managerId).Get((context.Background()))
	if err != nil {
		return err
	}
	err = res.DataTo(u)
	if err != nil {
		return err
	}
	return err
}
