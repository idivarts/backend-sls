package trendlymodels

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
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

	// Entitlements is the resolved, denormalized plan capability set (from
	// PlanLimitsMap[planKey]) mirrored onto the doc so the frontend can gate UI
	// without re-deriving it. Refreshed whenever planKey changes.
	Entitlements *OrgEntitlements `json:"entitlements,omitempty" firestore:"entitlements,omitempty"`

	// TokenWallet is the single shared AI-token wallet for the whole org — every
	// brand draws from one balance. Metered by real OpenRouter usage; refilled
	// monthly (see token_wallet.go + the Credit ticket §5b).
	TokenWallet *OrgTokenWallet `json:"tokenWallet,omitempty" firestore:"tokenWallet,omitempty"`

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

// OrgPlanLimits maps a plan tier to its hard entitlements + AI-token allotment.
// MonthlyAllotment is in model-weighted AI tokens (baseline = Gemini 3.5 Flash,
// see token_wallet.go). The Credit ticket pricing table (§5/§5b) is the source
// of truth for every value here.
type OrgPlanLimits struct {
	MaxBrands        int
	MaxSeats         int
	MonthlyAllotment int64  // AI tokens refilled on the 1st of each month
	AnalyticsTier    string // "locked" | "standard" | "full"
	Approvals        bool   // approvals / campaigns feature
	InboxReply       bool   // Combined Social Inbox: false = view-only (free tier)
	MaxPostsPerMonth int    // posting/scheduling cap; -1 = unlimited
}

// PlanLimitsMap is the single source of truth for plan -> entitlements + token
// allotment. New USD tiers come from the Credit ticket pricing table (§5/§5b);
// legacy India plan keys are kept so migrated orgs resolve sane limits during
// backfill (mapped onto the nearest new tier).
var PlanLimitsMap = map[string]OrgPlanLimits{
	// New USD tiers (Credit ticket).
	"free":   {MaxBrands: 1, MaxSeats: 1, MonthlyAllotment: 200_000, AnalyticsTier: "locked", Approvals: false, InboxReply: false, MaxPostsPerMonth: 2},
	"pro":    {MaxBrands: 1, MaxSeats: 2, MonthlyAllotment: 2_000_000, AnalyticsTier: "standard", Approvals: false, InboxReply: true, MaxPostsPerMonth: -1},
	"team":   {MaxBrands: 3, MaxSeats: 5, MonthlyAllotment: 5_000_000, AnalyticsTier: "full", Approvals: true, InboxReply: true, MaxPostsPerMonth: -1},
	"agency": {MaxBrands: 100, MaxSeats: 100, MonthlyAllotment: 20_000_000, AnalyticsTier: "full", Approvals: true, InboxReply: true, MaxPostsPerMonth: -1},
	// Legacy India tiers (backfill only).
	"starter":    {MaxBrands: 1, MaxSeats: 2, MonthlyAllotment: 2_000_000, AnalyticsTier: "standard", Approvals: false, InboxReply: true, MaxPostsPerMonth: -1},
	"growth":     {MaxBrands: 3, MaxSeats: 5, MonthlyAllotment: 5_000_000, AnalyticsTier: "full", Approvals: true, InboxReply: true, MaxPostsPerMonth: -1},
	"enterprise": {MaxBrands: 100, MaxSeats: 100, MonthlyAllotment: 20_000_000, AnalyticsTier: "full", Approvals: true, InboxReply: true, MaxPostsPerMonth: -1},
}

// ResolvePlanLimits returns the full limits for a plan key, defaulting to the
// free tier for unknown/empty keys.
func ResolvePlanLimits(planKey string) OrgPlanLimits {
	if v, ok := PlanLimitsMap[planKey]; ok {
		return v
	}
	return PlanLimitsMap["free"]
}

// OrgEntitlements is the denormalized capability set written onto the org doc
// (resolved from PlanLimitsMap[planKey]) so clients can gate UI directly without
// re-deriving the plan rules.
type OrgEntitlements struct {
	MaxBrands        int    `json:"maxBrands" firestore:"maxBrands"`
	MaxSeats         int    `json:"maxSeats" firestore:"maxSeats"`
	AnalyticsTier    string `json:"analyticsTier" firestore:"analyticsTier"`
	Approvals        bool   `json:"approvals" firestore:"approvals"`
	InboxReply       bool   `json:"inboxReply" firestore:"inboxReply"`
	MaxPostsPerMonth int    `json:"maxPostsPerMonth" firestore:"maxPostsPerMonth"`
}

// EntitlementsFor resolves a plan key to its denormalized entitlements (written
// onto the org doc on create / plan change).
func EntitlementsFor(planKey string) *OrgEntitlements {
	l := ResolvePlanLimits(planKey)
	return &OrgEntitlements{
		MaxBrands:        l.MaxBrands,
		MaxSeats:         l.MaxSeats,
		AnalyticsTier:    l.AnalyticsTier,
		Approvals:        l.Approvals,
		InboxReply:       l.InboxReply,
		MaxPostsPerMonth: l.MaxPostsPerMonth,
	}
}

// NextMonthlyReset returns epoch-ms of the first day of the NEXT month at 00:00
// UTC — the anchor the token wallet renews on (billing is anchored to the 1st).
func NextMonthlyReset(now time.Time) int64 {
	return time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
}

// ApplyPlanToOrg sets an org's plan end-to-end: writes planKey + maxBrands +
// resolved entitlements onto the org doc, then refills the token wallet to the
// plan's monthly allotment (resetting on periodResetAt). Called when a
// subscription becomes active / changes plan, when an invoice is paid, and on
// org creation (free plan). Idempotent for a billing cycle: re-applying the same
// active plan in a new month simply refills the wallet for that month.
func ApplyPlanToOrg(orgID, planKey string, periodResetAt int64) error {
	limits := ResolvePlanLimits(planKey)
	if _, err := firestoredb.Client.Collection(orgCollection).Doc(orgID).Update(context.Background(), []firestore.Update{
		{Path: "planKey", Value: planKey},
		{Path: "maxBrands", Value: limits.MaxBrands},
		{Path: "entitlements", Value: EntitlementsFor(planKey)},
	}); err != nil {
		return err
	}
	return RefillWallet(orgID, limits.MonthlyAllotment, periodResetAt)
}

// ListAllOrganizations returns every organization document (including soft-
// deleted ones — the caller filters on DeletedAt). Used by the monthly billing
// cron, which scans all orgs to renew wallets + advance the billing state.
func ListAllOrganizations() ([]OrganizationWithID, error) {
	docs, err := firestoredb.Client.Collection(orgCollection).Documents(context.Background()).GetAll()
	if err != nil {
		return nil, err
	}
	out := make([]OrganizationWithID, 0, len(docs))
	for _, d := range docs {
		var o Organization
		if err := d.DataTo(&o); err != nil {
			continue
		}
		out = append(out, OrganizationWithID{ID: d.Ref.ID, Organization: o})
	}
	return out, nil
}

// SetOrgAccessState updates the org-level billing access state
// ("active" | "past_due" | "locked" | "canceled") that the paywall/lock reads.
func SetOrgAccessState(orgID, state string) error {
	_, err := firestoredb.Client.Collection(orgCollection).Doc(orgID).
		Update(context.Background(), []firestore.Update{{Path: "billing.accessState", Value: state}})
	return err
}

// ResolveMaxBrands returns the brand cap for a plan key, defaulting to 1 (the
// free-tier cap) for unknown/empty keys.
func ResolveMaxBrands(planKey string) int {
	if v, ok := PlanLimitsMap[planKey]; ok {
		return v.MaxBrands
	}
	return 1
}

// ResolveMaxSeats returns the org member (seat) cap for a plan key, defaulting
// to 1 (the free-tier cap) for unknown/empty keys.
func ResolveMaxSeats(planKey string) int {
	if v, ok := PlanLimitsMap[planKey]; ok {
		return v.MaxSeats
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

// CountOrgMembers returns the number of member docs in the org's orgMembers
// subcollection. Every doc (any role/status) holds a seat, so this is the count
// the plan's MaxSeats cap is enforced against.
func CountOrgMembers(orgID string) (int, error) {
	docs, err := firestoredb.Client.Collection(orgCollection).Doc(orgID).
		Collection("orgMembers").Documents(context.Background()).GetAll()
	if err != nil {
		return 0, err
	}
	return len(docs), nil
}

// DeleteOrgMember removes a manager's membership from the org.
func DeleteOrgMember(orgID, managerID string) error {
	_, err := firestoredb.Client.Collection(orgCollection).Doc(orgID).
		Collection("orgMembers").Doc(managerID).Delete(context.Background())
	return err
}

