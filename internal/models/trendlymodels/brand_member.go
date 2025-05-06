package trendlymodels

import (
	"context"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"google.golang.org/api/iterator"
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

func GetAllBrandMembers(brandID string) ([]BrandMember, error) {
	var members []BrandMember

	iter := firestoredb.Client.Collection("brands").Doc(brandID).Collection("members").Documents(context.Background())
	defer iter.Stop()

	for {
		doc, err := iter.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			return nil, err
		}

		var member BrandMember
		if err := doc.DataTo(&member); err != nil {
			return nil, err
		}

		members = append(members, member)
	}

	return members, nil
}
