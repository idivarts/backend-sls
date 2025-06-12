package trendlymodels

import (
	"context"

	"cloud.google.com/go/firestore"
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

	Moderations struct {
		BlockedInfluencers  []string `json:"blockedInfluencers,omitempty" firestore:"blockedInfluencers,omitempty"`
		ReportedInfluencers []string `json:"reportedInfluencers,omitempty" firestore:"reportedInfluencers,omitempty"`
	} `json:"moderations,omitempty" firestore:"moderations,omitempty"`

	CreationTime int64 `json:"creationTime" firestore:"creationTime"`
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

func (u *Manager) Insert(managerId string) (*firestore.WriteResult, error) {
	wr, err := firestoredb.Client.Collection("managers").Doc(managerId).Set(context.Background(), u)
	return wr, err
}
