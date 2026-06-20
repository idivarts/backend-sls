package trendlymodels

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"google.golang.org/api/iterator"
)

type Brand struct {
	Name                  string  `json:"name" firestore:"name"`
	Image                 *string `json:"image,omitempty" firestore:"image,omitempty"`
	PaymentMethodVerified *bool   `json:"paymentMethodVerified,omitempty" firestore:"paymentMethodVerified,omitempty"`

	// OrganizationID is the parent org this brand belongs to. Brands created
	// before the Organization rollout have no value here until backfilled.
	OrganizationID *string `json:"organizationId,omitempty" firestore:"organizationId,omitempty"`

	// DeletedAt soft-deletes the brand (epoch ms). Non-nil means deleted; such
	// brands are removed from their org's brandIds and excluded from surfaces.
	DeletedAt *int64 `json:"deletedAt,omitempty" firestore:"deletedAt,omitempty"`

	// Country is the brand's ISO-3166 alpha-2 country code (e.g. "IN", "US"),
	// captured silently from the client at onboarding. It is the source of truth
	// for India-only gating (discovery, in-app invites, Razorpay payments).
	// Legacy brands have no value here and MUST be treated as India — use
	// IsIndia() rather than comparing this field directly.
	Country *string `json:"country,omitempty" firestore:"country,omitempty"`

	Profile *BrandProfile `json:"profile,omitempty" firestore:"profile,omitempty"`

	Preferences         *BrandPreferences    `json:"preferences,omitempty" firestore:"preferences,omitempty"`
	DiscoverPreferences *DiscoverPreferences `json:"discoverPreferences,omitempty" firestore:"discoverPreferences,omitempty"`

	Backend *BrandBackend `json:"backend,omitempty" firestore:"backend,omitempty"`
	Survey  *BrandSurvey  `json:"survey,omitempty" firestore:"survey,omitempty"`

	// Billing/subscription lives on the Organization (see Organization.Billing).
	// Read it via brand.OrganizationID → Organization.Get → Organization.Billing.
	// Legacy Firestore docs may still carry `billing` and `isBillingDisabled`;
	// those fields are ignored.

	UnlockedInfluencers   []string               `json:"unlockedInfluencers,omitempty" firestore:"unlockedInfluencers,omitempty"`
	DiscoveredInfluencers []string               `json:"discoveredInfluencers,omitempty" firestore:"discoveredInfluencers,omitempty"`
	ConnectedInfluencers  BrandInfluencerConnect `json:"connectedInfluencers" firestore:"connectedInfluencers"`
	PostedCollaborations  []string               `json:"postedCollaborations,omitempty" firestore:"postedCollaborations,omitempty"`

	// NOTE: the old per-brand `Credits` (BrandCredits, 5 buckets) is removed.
	// Billing/subscription moves to the Organization, and the new credit system
	// is a single org-level token wallet (see the Credit System ticket). Legacy
	// Firestore docs may still carry a `credits` map; it is simply ignored.

	HasPayWall bool `json:"hasPayWall" firestore:"hasPayWall"`

	// Age is the brand's maturity bucket collected during onboarding
	// ("JUST_STARTING" | "LT_1" | "LT_5" | "GT_5").
	Age *string `json:"age,omitempty" firestore:"age,omitempty"`

	// OnboardingComplete is false for a draft brand created at the start of the
	// AI onboarding chat and flipped to true only when onboarding finishes and
	// the brand is provisioned (billing/credits/team). Draft brands must be
	// excluded from billing/discover/list surfaces until this is true.
	OnboardingComplete bool `json:"onboardingComplete" firestore:"onboardingComplete"`

	AIVoice *string `json:"aiVoice,omitempty" firestore:"aiVoice,omitempty"`

	// AIMemory is a per-brand, AI-maintained note of durable brand facts the user
	// has shared in chat (positioning, audience, voice, products, do's & don'ts,
	// recurring preferences). It is pre-fed into the system prompt of EVERY AI
	// conversation for this brand so the user never re-explains context. The AI
	// appends to it via the update_brand_memory tool; the user can also edit it
	// directly on the brand-profile page. Scoped strictly to this brand (never
	// shared across brands or organizations) and kept under a char cap via LLM
	// compaction when it grows too large.
	AIMemory          *string `json:"aiMemory,omitempty" firestore:"aiMemory,omitempty"`
	AIMemoryUpdatedAt *int64  `json:"aiMemoryUpdatedAt,omitempty" firestore:"aiMemoryUpdatedAt,omitempty"`

	// Members       []BrandMember  `json:"members" firestore:"members"`
	// Notifications []Notification `json:"notifications" firestore:"notifications"`
}
type BrandInfluencerConnect struct {
	Requested []string `json:"requested,omitempty" firestore:"requested,omitempty"`
	Connected []string `json:"connected,omitempty" firestore:"connected,omitempty"`
}
type BrandBilling struct {
	Subscription    *string `json:"subscription,omitempty" firestore:"subscription,omitempty"`
	PaymentLinkId   *string `json:"paymentLinkId,omitempty" firestore:"paymentLinkId,omitempty"`
	SubscriptionUrl *string `json:"subscriptionUrl,omitempty" firestore:"subscriptionUrl,omitempty"`
	BillingStatus   *string `json:"billingStatus,omitempty" firestore:"billingStatus,omitempty"`
	// IsGrowthPlan  *bool   `json:"isGrowthPlan,omitempty" firestore:"isGrowthPlan,omitempty"`
	PlanKey   *string `json:"planKey,omitempty" firestore:"planKey,omitempty"`
	PlanCycle *string `json:"planCycle,omitempty" firestore:"planCycle,omitempty"`
	IsOnTrial *bool   `json:"isOnTrial,omitempty" firestore:"isOnTrial,omitempty"`
	TrialEnds *int64  `json:"trialEnds,omitempty" firestore:"trialEnds,omitempty"`
	EndsAt    *int64  `json:"endsAt,omitempty" firestore:"endsAt,omitempty"`
	Status    *int    `json:"status,omitempty" firestore:"status,omitempty"`

	// ── Org-level USD billing state machine (Credit ticket §5a/§6) ──
	// These layer the new access-control shape on top of the legacy Razorpay
	// fields above without breaking existing webhook code. `BillingStatus` still
	// holds the raw Razorpay status; `AccessState` is OUR app-level state that the
	// paywall/lock + cron drive.
	Provider           *string `json:"provider,omitempty" firestore:"provider,omitempty"`                     // "razorpay" now; future MoR
	AccessState        *string `json:"accessState,omitempty" firestore:"accessState,omitempty"`               // "active" | "past_due" | "locked" | "canceled"
	BillingMode        *string `json:"billingMode,omitempty" firestore:"billingMode,omitempty"`               // "recurring" | "invoice"
	BillingAnchorDay   *int    `json:"billingAnchorDay,omitempty" firestore:"billingAnchorDay,omitempty"`     // always 1
	PeriodEnd          *int64  `json:"periodEnd,omitempty" firestore:"periodEnd,omitempty"`                   // end of current paid month (next 1st)
	ProratedFirstMonth *bool   `json:"proratedFirstMonth,omitempty" firestore:"proratedFirstMonth,omitempty"`
}

type BrandProfile struct {
	About       *string  `json:"about,omitempty" firestore:"about,omitempty"`
	Banner      *string  `json:"banner,omitempty" firestore:"banner,omitempty"`
	Industries  []string `json:"industries,omitempty" firestore:"industries,omitempty"`
	Website     *string  `json:"website,omitempty" firestore:"website,omitempty"`
	PhoneNumber *string  `json:"phone,omitempty" firestore:"phone,omitempty"`
}

type BrandPreferences struct {
	PromotionType          []string `json:"promotionType,omitempty" firestore:"promotionType,omitempty"`
	InfluencerCategories   []string `json:"influencerCategories,omitempty" firestore:"influencerCategories,omitempty"`
	Languages              []string `json:"languages,omitempty" firestore:"languages,omitempty"`
	Locations              []string `json:"locations,omitempty" firestore:"locations,omitempty"`
	Platforms              []string `json:"platforms,omitempty" firestore:"platforms,omitempty"`
	CollaborationPostTypes []string `json:"collaborationPostTypes,omitempty" firestore:"collaborationPostTypes,omitempty"`
	TimeCommitments        []string `json:"timeCommitments,omitempty" firestore:"timeCommitments,omitempty"`
	ContentVideoType       []string `json:"contentVideoType,omitempty" firestore:"contentVideoType,omitempty"`
}

type BrandBackend struct {
	HireRate *float64 `json:"hireRate,omitempty" firestore:"hireRate,omitempty"`
}

type BrandSurvey struct {
	Source             *string `json:"source,omitempty" firestore:"source,omitempty"`
	Purpose            *string `json:"purpose,omitempty" firestore:"purpose,omitempty"`
	CollaborationValue *string `json:"collaborationValue,omitempty" firestore:"collaborationValue,omitempty"`
}

// IsIndia reports whether the brand should be treated as India-based. A brand
// with no Country set (all legacy brands) is treated as India for backward
// compatibility. Comparison is case-insensitive on the alpha-2 code "IN".
func (b *Brand) IsIndia() bool {
	if b.Country == nil || *b.Country == "" {
		return true
	}
	return strings.EqualFold(*b.Country, "IN")
}

func (u *Brand) Get(brandId string) error {
	res, err := firestoredb.Client.Collection("brands").Doc(brandId).Get((context.Background()))
	if err != nil {
		return err
	}
	err = res.DataTo(u)
	if err != nil {
		return err
	}
	return err
}

// UpdateBrandFields applies a partial update to a brand document. Callers build
// the []firestore.Update (supports nested FieldPath updates like
// "profile.phone"); the Firestore call itself lives here in the model.
func UpdateBrandFields(ctx context.Context, brandID string, updates []firestore.Update) error {
	_, err := firestoredb.Client.Collection("brands").Doc(brandID).Update(ctx, updates)
	return err
}

// SetBrandMemory replaces the brand's AI memory blob and stamps the update time.
// Used by the update_brand_memory AI tool; the user-facing direct edit on the
// brand-profile page goes through the client Firestore write path instead.
func SetBrandMemory(ctx context.Context, brandID, memory string) error {
	return UpdateBrandFields(ctx, brandID, []firestore.Update{
		{Path: "aiMemory", Value: memory},
		{Path: "aiMemoryUpdatedAt", Value: time.Now().UnixMilli()},
	})
}

// HardDeleteBrand permanently removes the brand document, every nested
// subcollection beneath it, AND every collaboration owned by the brand
// (collaborations live at the top level, keyed by brandId, with their own
// applications/invitations subcollections). Walks subcollections recursively
// because brands own a large fan-out (members, teams, socials, strategies/*,
// contents/*, inbox, analytics caches, calendar comments, etc.) and the
// Firestore SDK has no built-in recursive delete. Uses a BulkWriter so each
// level's deletes run in parallel. The caller must have already verified the
// brand has no active contracts — terminal contracts are intentionally left
// as historical records.
func HardDeleteBrand(brandId string) error {
	ctx := context.Background()

	if err := deleteCollaborationsForBrand(ctx, brandId); err != nil {
		return err
	}

	docRef := firestoredb.Client.Collection("brands").Doc(brandId)
	return deleteDocRecursive(ctx, docRef)
}

// deleteCollaborationsForBrand wipes every top-level collaboration owned by
// the brand, including the applications/invitations subcollections under each.
func deleteCollaborationsForBrand(ctx context.Context, brandId string) error {
	iter := firestoredb.Client.Collection("collaborations").
		Where("brandId", "==", brandId).Documents(ctx)
	defer iter.Stop()
	for {
		snap, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		if err := deleteDocRecursive(ctx, snap.Ref); err != nil {
			return err
		}
	}
	return nil
}

func deleteDocRecursive(ctx context.Context, doc *firestore.DocumentRef) error {
	subIter := doc.Collections(ctx)
	for {
		sub, err := subIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		if err := deleteCollectionRecursive(ctx, sub); err != nil {
			return err
		}
	}
	_, err := doc.Delete(ctx)
	return err
}

func deleteCollectionRecursive(ctx context.Context, col *firestore.CollectionRef) error {
	bw := firestoredb.Client.BulkWriter(ctx)
	iter := col.Documents(ctx)
	defer iter.Stop()
	for {
		snap, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			bw.End()
			return err
		}
		if err := deleteDocChildren(ctx, snap.Ref); err != nil {
			bw.End()
			return err
		}
		if _, err := bw.Delete(snap.Ref); err != nil {
			bw.End()
			return err
		}
	}
	bw.End()
	return nil
}

func deleteDocChildren(ctx context.Context, doc *firestore.DocumentRef) error {
	subIter := doc.Collections(ctx)
	for {
		sub, err := subIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		if err := deleteCollectionRecursive(ctx, sub); err != nil {
			return err
		}
	}
	return nil
}

func (b *Brand) Insert(brandId string) (*firestore.WriteResult, error) {
	// Marshal the struct to JSON
	bytes, err := json.Marshal(b)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal user: %w", err)
	}

	// Unmarshal into a map
	var data map[string]interface{}
	if err := json.Unmarshal(bytes, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal to map: %w", err)
	}

	res, err := firestoredb.Client.Collection("brands").Doc(brandId).Set(context.Background(), data, firestore.MergeAll)

	if err != nil {
		return nil, err
	}
	return res, err
}
