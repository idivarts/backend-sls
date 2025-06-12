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

func GetMyBrandMemberships(managerId string) ([]BrandMember, error) {
	var members []BrandMember

	iter := firestoredb.Client.CollectionGroup("members").Where("managerId", "==", managerId).Documents(context.Background())
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

func GetMyBrands(managerId string) ([]Brand, error) {
	var brands []Brand

	brandIds := []string{}

	iter := firestoredb.Client.CollectionGroup("members").Where("managerId", "==", managerId).Documents(context.Background())
	defer iter.Stop()

	for {
		doc, err := iter.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			return nil, err
		}

		brandId := doc.Ref.Parent.Parent.ID
		brandIds = append(brandIds, brandId)
	}

	iter2 := firestoredb.Client.Collection("brands").Where(firestore.DocumentID, "in", brandIds).Documents(context.Background())
	defer iter2.Stop()
	for {
		doc, err := iter2.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			return nil, err
		}

		var brand Brand
		if err := doc.DataTo(&brand); err != nil {
			return nil, err
		}

		brands = append(brands, brand)
	}

	return brands, nil
}
