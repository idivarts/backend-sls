package trendlymodels

import (
	"context"

	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

type Contract struct {
	UserID          string `json:"userId" firestore:"userId"`
	ManagerID       string `json:"managerId" firestore:"managerId"`
	CollaborationID string `json:"collaborationId" firestore:"collaborationId"`
	BrandID         string `json:"brandId" firestore:"brandId"`
	StreamChannelID string `json:"streamChannelId" firestore:"streamChannelId"`
	Status          int    `json:"status" firestore:"status"`

	// All Items for storing the monetization related data
}

func (b *Contract) Get(contractID string) error {
	res, err := firestoredb.Client.Collection("contracts").Doc(contractID).Get(context.Background())
	if err != nil {
		return err
	}

	err = res.DataTo(b)
	if err != nil {
		return err
	}
	return err
}

func (b *Contract) GetByCollab(collabId, userId string) error {
	iter := firestoredb.Client.Collection("contracts").Where("collaborationId", "==", collabId).Where("userId", "==", userId).Documents(context.Background())

	res, err := iter.Next()
	if err != nil {
		return err
	}

	err = res.DataTo(b)
	if err != nil {
		return err
	}
	return err
}
