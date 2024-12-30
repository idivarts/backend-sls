package trendlymodels

import (
	"context"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

type BrandMember struct {
	ManagerID string `json:"managerId" firestore:"managerId"`
	Role      string `json:"role" firestore:"role"`
	Status    int    `json:"status" firestore:"status"`
}

func (b *BrandMember) Set(brandID string) (*firestore.WriteResult, error) {
	res, err := firestoredb.Client.Collection("brands").Doc(brandID).Collection("members").Doc(b.ManagerID).Set(context.Background(), b)
	return res, err
}

func (b *BrandMember) Get(brandID, userID string) error {
	res, err := firestoredb.Client.Collection("brands").Doc(brandID).Collection("members").Doc(userID).Get(context.Background())
	if err != nil {
		return err
	}

	err = res.DataTo(b)
	if err != nil {
		return err
	}
	return err
}
