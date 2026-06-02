package trendlymodels

import (
	"context"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"google.golang.org/api/iterator"
)

type BrandMember struct {
	ManagerID string    `json:"managerId" firestore:"managerId"`
	Role      BrandRole `json:"role" firestore:"role"`
	Status    int       `json:"status" firestore:"status"`
	// TeamIDs scopes which teams (and therefore which socials/collabs) this
	// member can act on. Empty means the brand's default team only.
	TeamIDs []string `json:"teamIds,omitempty" firestore:"teamIds,omitempty"`
	// Overrides are per-member capability toggles that win over the role
	// default. Keys are Capability strings; only OverridableCapabilities are
	// honoured. Stored as map[string]bool for Firestore compatibility.
	Overrides map[string]bool `json:"overrides,omitempty" firestore:"overrides,omitempty"`
}

// HasCapability resolves whether this member effectively holds cap. Resolution
// order: Owner has everything; an explicit per-member override wins next; the
// role default applies otherwise.
func (b *BrandMember) HasCapability(cap Capability) bool {
	if b.Role == RoleOwner {
		return true
	}
	if b.Overrides != nil {
		if v, ok := b.Overrides[string(cap)]; ok {
			return v
		}
	}
	return roleCapabilities[b.Role][cap]
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
