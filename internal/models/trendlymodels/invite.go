package trendlymodels

import (
	"context"

	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

type Invitation struct {
	UserID          string `json:"userId" firestore:"userId"`
	CollaborationID string `json:"collaborationId" firestore:"collaborationId"`
	ManagerID       string `json:"managerId" firestore:"managerId"`
	Status          string `json:"status" firestore:"status"`
	TimeStamp       int64  `json:"timeStamp" firestore:"timeStamp"`
	Message         string `json:"message" firestore:"message"`
}

func (b *Invitation) Get(collabID, userID string) error {
	res, err := firestoredb.Client.Collection("collaborations").Doc(collabID).Collection("invitations").Doc(userID).Get(context.Background())
	if err != nil {
		return err
	}

	err = res.DataTo(b)
	if err != nil {
		return err
	}
	return err
}
