package trendlymodels

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"github.com/idivarts/backend-sls/pkg/messenger"
)

type Socials struct {
	ID           string                      `json:"id" firestore:"id"`
	Name         string                      `json:"name" firestore:"name"`
	Image        string                      `json:"image" firestore:"image"`
	IsInstagram  bool                        `json:"isInstagram" firestore:"isInstagram"`
	ConnectedID  *string                     `json:"connectedId,omitempty" firestore:"connectedId"`
	UserID       string                      `json:"userId" firestore:"userId"`
	OwnerName    string                      `json:"ownerName" firestore:"ownerName"`
	InstaProfile *messenger.InstagramProfile `json:"instaProfile,omitempty" firestore:"instaProfile"`
	FBProfile    *messenger.FacebookProfile  `json:"fbProfile,omitempty" firestore:"fbProfile"`
}

type SocialsPrivate struct {
	AccessToken *string `json:"accessToken,omitempty" firestore:"accessToken"`
}

func (s *SocialsPrivate) Set(userId, id string) (*firestore.WriteResult, error) {
	res, err := firestoredb.Client.Collection(fmt.Sprintf("users/%s/socialsPrivate", userId)).Doc(id).Set(context.Background(), s)

	if err != nil {
		return nil, err
	}
	return res, err
}

func (s *Socials) Insert(userId string) (*firestore.WriteResult, error) {
	res, err := firestoredb.Client.Collection(fmt.Sprintf("users/%s/socials", userId)).Doc(s.ID).Set(context.Background(), s)

	if err != nil {
		return nil, err
	}
	return res, err
}
