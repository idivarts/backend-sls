package trendlymodels

import (
	"context"

	"cloud.google.com/go/firestore"
	"github.com/google/uuid"
	"github.com/idivarts/backend-sls/internal/models/trendlybq"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

type Invitation struct {
	UserID     string `json:"userId" firestore:"userId"`
	IsDiscover bool   `json:"isDiscover" firestore:"isDiscover"`

	CollaborationID string `json:"collaborationId" firestore:"collaborationId"`
	ManagerID       string `json:"managerId" firestore:"managerId"`

	SocialProfile *trendlybq.SocialsBreif `json:"socialProfile,omitempty" firestore:"socialProfile,omitempty"`

	Status    string `json:"status" firestore:"status"`
	TimeStamp int64  `json:"timeStamp" firestore:"timeStamp"`
	Message   string `json:"message" firestore:"message"`
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

func (data *Invitation) getID() string {
	ID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(data.CollaborationID+"-"+data.UserID))
	return ID.String()
}

func (b *Invitation) Update() (*firestore.WriteResult, error) {
	res, err := firestoredb.Client.Collection("collaborations-invites").Doc(b.getID()).Set(context.Background(), b)
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
	doc := firestoredb.Client.Collection("collaborations-invites").Doc(b.getID())

	// Try to create first; this will fail with AlreadyExists if the doc is present.
	if res, err := doc.Create(ctx, b); err == nil {
		return res, nil
	} else {
		return nil, err
	}
}
