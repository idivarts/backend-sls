package trendlymodels

import (
	"context"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"google.golang.org/api/iterator"
)

type BrandMember struct {
	ManagerID string `json:"managerId" firestore:"managerId"`
	Status    int    `json:"status" firestore:"status"`
	// TeamID is the single team this member belongs to. The member inherits that
	// team's feature privileges. Empty only for members not yet migrated to the
	// team-privilege model (see scripts/migrate-teams-v2).
	TeamID string `json:"teamId,omitempty" firestore:"teamId,omitempty"`
}

// ResolveTeam loads the team this member belongs to. Returns (nil, nil) when the
// member has no team assigned (pre-migration legacy member).
func (b *BrandMember) ResolveTeam(brandID string) (*Team, error) {
	if b.TeamID == "" {
		return nil, nil
	}
	team := &Team{}
	if err := team.Get(brandID, b.TeamID); err != nil {
		return nil, err
	}
	return team, nil
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

func DeleteBrandMember(brandID, managerID string) error {
	_, err := firestoredb.Client.Collection("brands").Doc(brandID).Collection("members").Doc(managerID).Delete(context.Background())
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

func GetMyFirstBrand(managerId string) (*Brand, error) {
	var brand Brand

	iter := firestoredb.Client.CollectionGroup("members").Where("managerId", "==", managerId).Limit(1).Documents(context.Background())
	defer iter.Stop()

	doc, err := iter.Next()
	if err != nil {
		return nil, err
	}
	brandId := doc.Ref.Parent.Parent.ID

	doc, err = firestoredb.Client.Collection("brands").Doc(brandId).Get(context.Background())
	if err != nil {
		return nil, err
	}

	if err := doc.DataTo(&brand); err != nil {
		return nil, err
	}
	return &brand, nil
}
