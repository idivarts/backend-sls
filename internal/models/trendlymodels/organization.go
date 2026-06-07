package trendlymodels

import (
	"context"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"google.golang.org/api/iterator"
)

// Organization is the new top-level tenant that sits ABOVE Brand. An org owns a
// set of brands (capped by its plan), and is the single billing/subscription
// entity (billing moves Brand -> Org). The old per-brand credit system is being
// removed; the new org-level token wallet is added later by the Credit ticket,
// so there is intentionally NO credits field here yet.
type Organization struct {
	Name    string  `json:"name" firestore:"name"`
	Image   *string `json:"image,omitempty" firestore:"image,omitempty"`
	OwnerID string  `json:"ownerId" firestore:"ownerId"`

	// BrandIds is the denormalized list of (non-deleted) brands in this org. It
	// is the source of truth for the maxBrands cap + grouped switcher, and is
	// kept clean: deleting/transferring a brand removes its id here.
	BrandIds []string `json:"brandIds" firestore:"brandIds"`

	// Billing is the org-level billing shell (moved from Brand.Billing). The
	// full USD/token-wallet shape is layered on later by the Credit ticket.
	Billing *OrgBilling `json:"billing,omitempty" firestore:"billing,omitempty"`

	// PlanKey drives maxBrands (+ entitlements/token allotment via the Credit
	// ticket). New tiers: "free" | "pro" | "team" | "agency".
	PlanKey   *string `json:"planKey,omitempty" firestore:"planKey,omitempty"`
	MaxBrands int     `json:"maxBrands" firestore:"maxBrands"`

	CreationTime int64 `json:"creationTime" firestore:"creationTime"`

	// DeletedAt soft-deletes the org (epoch ms). Non-nil means deleted; such
	// orgs are excluded from list/get surfaces.
	DeletedAt *int64 `json:"deletedAt,omitempty" firestore:"deletedAt,omitempty"`
}

// OrgBilling is the org billing shell. It mirrors BrandBilling for now (the
// billing entity simply moves up to the org); the Credit ticket evolves this
// into the USD/Razorpay-International + token-wallet model. Aliased so existing
// billing code can be re-pointed with minimal churn.
type OrgBilling = BrandBilling

// OrganizationWithID is an Organization plus its Firestore doc id, for API
// responses (the bare struct does not carry its own id).
type OrganizationWithID struct {
	ID string `json:"id"`
	Organization
}

type OrgRole string

const (
	OrgRoleOwner  OrgRole = "org_owner"
	OrgRoleAdmin  OrgRole = "org_admin"
	OrgRoleMember OrgRole = "member"
)

// OrganizationMember lives at organizations/{orgId}/orgMembers/{managerId}. The
// subcollection is deliberately named "orgMembers" (NOT "members") so it does
// not collide with the brands/{brandId}/members collection-group queries used
// by the brand switcher.
type OrganizationMember struct {
	ManagerID string  `json:"managerId" firestore:"managerId"`
	Role      OrgRole `json:"role" firestore:"role"`
	Status    int     `json:"status" firestore:"status"`
}

// OrgPlanLimits maps a plan tier to its hard entitlements owned by the Org
// ticket. Token/credit allotments are layered onto the same keys by the Credit
// ticket.
type OrgPlanLimits struct {
	MaxBrands int
	MaxSeats  int
}

// PlanLimitsMap is the single source of truth for plan -> brand/seat caps.
// New USD tiers come from the Credit ticket pricing table; legacy India plan
// keys are kept so migrated orgs resolve a sane cap during backfill.
var PlanLimitsMap = map[string]OrgPlanLimits{
	// New USD tiers (Credit ticket).
	"free":   {MaxBrands: 1, MaxSeats: 1},
	"pro":    {MaxBrands: 1, MaxSeats: 2},
	"team":   {MaxBrands: 3, MaxSeats: 5},
	"agency": {MaxBrands: 100, MaxSeats: 100},
	// Legacy India tiers (backfill only).
	"starter":    {MaxBrands: 1, MaxSeats: 2},
	"growth":     {MaxBrands: 3, MaxSeats: 5},
	"enterprise": {MaxBrands: 100, MaxSeats: 100},
}

// ResolveMaxBrands returns the brand cap for a plan key, defaulting to 1 (the
// free-tier cap) for unknown/empty keys.
func ResolveMaxBrands(planKey string) int {
	if v, ok := PlanLimitsMap[planKey]; ok {
		return v.MaxBrands
	}
	return 1
}

const orgCollection = "organizations"

func (o *Organization) Get(orgID string) error {
	res, err := firestoredb.Client.Collection(orgCollection).Doc(orgID).Get(context.Background())
	if err != nil {
		return err
	}
	return res.DataTo(o)
}

// Insert creates a new organization doc with an auto-generated id and returns
// that id.
func (o *Organization) Insert() (string, error) {
	ref := firestoredb.Client.Collection(orgCollection).NewDoc()
	if _, err := ref.Set(context.Background(), o); err != nil {
		return "", err
	}
	return ref.ID, nil
}

// SetBilling writes just the billing field on the org doc (used by billing
// webhooks; billing is the org's responsibility now).
func (o *Organization) SetBilling(orgID string, billing *OrgBilling) error {
	_, err := firestoredb.Client.Collection(orgCollection).Doc(orgID).
		Update(context.Background(), []firestore.Update{{Path: "billing", Value: billing}})
	return err
}

func (m *OrganizationMember) Set(orgID string) (*firestore.WriteResult, error) {
	return firestoredb.Client.Collection(orgCollection).Doc(orgID).
		Collection("orgMembers").Doc(m.ManagerID).Set(context.Background(), m)
}

func (m *OrganizationMember) Get(orgID, managerID string) error {
	res, err := firestoredb.Client.Collection(orgCollection).Doc(orgID).
		Collection("orgMembers").Doc(managerID).Get(context.Background())
	if err != nil {
		return err
	}
	return res.DataTo(m)
}

// GetMyOrganizations returns every non-deleted organization the manager is a
// member of (across all orgs), each with its doc id.
func GetMyOrganizations(managerID string) ([]OrganizationWithID, error) {
	orgs := []OrganizationWithID{}

	iter := firestoredb.Client.CollectionGroup("orgMembers").
		Where("managerId", "==", managerID).Documents(context.Background())
	defer iter.Stop()

	for {
		doc, err := iter.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			return nil, err
		}

		orgID := doc.Ref.Parent.Parent.ID
		orgDoc, err := firestoredb.Client.Collection(orgCollection).Doc(orgID).Get(context.Background())
		if err != nil {
			// Membership doc orphaned (org hard-deleted) — skip rather than fail.
			continue
		}

		var org Organization
		if err := orgDoc.DataTo(&org); err != nil {
			return nil, err
		}
		if org.DeletedAt != nil {
			continue
		}
		orgs = append(orgs, OrganizationWithID{ID: orgID, Organization: org})
	}

	return orgs, nil
}
