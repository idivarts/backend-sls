package trendlymodels

import (
	"context"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

type Invitation struct {
	UserID     string `json:"userId" firestore:"userId"`
	IsDiscover bool   `json:"isDiscover" firestore:"isDiscover"`

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

func (b *Invitation) Insert() (*firestore.WriteResult, error) {
	res, err := firestoredb.Client.Collection("collaborations").Doc(b.CollaborationID).Collection("invitations").Doc(b.UserID).Set(context.Background(), b)
	if err != nil {
		return nil, err
	}

	return res, nil
}

// Upsert creates a new invitation if it does not exist; otherwise it updates the existing doc.
// Returns (writeResult, created=true) when a fresh document was inserted,
// or (writeResult, created=false) when an existing document was updated.
func (b *Invitation) Create() (*firestore.WriteResult, error) {
	ctx := context.Background()
	doc := firestoredb.Client.Collection("collaborations").Doc(b.CollaborationID).Collection("invitations").Doc(b.UserID)

	// Try to create first; this will fail with AlreadyExists if the doc is present.
	if res, err := doc.Create(ctx, b); err == nil {
		return res, nil
	} else {
		return nil, err
	}
}
