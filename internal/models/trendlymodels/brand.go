package trendlymodels

import (
	"context"
	"encoding/json"
	"fmt"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

type Brand struct {
	Name                  string  `json:"name" firestore:"name"`
	Image                 *string `json:"image,omitempty" firestore:"image,omitempty"`
	PaymentMethodVerified *bool   `json:"paymentMethodVerified,omitempty" firestore:"paymentMethodVerified,omitempty"`

	Profile     *BrandProfile     `json:"profile,omitempty" firestore:"profile,omitempty"`
	Preferences *BrandPreferences `json:"preferences,omitempty" firestore:"preferences,omitempty"`
	Backend     *BrandBackend     `json:"backend,omitempty" firestore:"backend,omitempty"`
	Survey      *BrandSurvey      `json:"survey,omitempty" firestore:"survey,omitempty"`

	IsBillingDisabled bool          `json:"isBillingDisabled" firestore:"isBillingDisabled"`
	Billing           *BrandBilling `json:"billing,omitempty" firestore:"billing,omitempty"`

	UnlockedInfluencers   []string               `json:"unlockedInfluencers,omitempty" firestore:"unlockedInfluencers,omitempty"`
	DiscoveredInfluencers []string               `json:"discoveredInfluencers,omitempty" firestore:"discoveredInfluencers,omitempty"`
	ConnectedInfluencers  BrandInfluencerConnect `json:"connectedInfluencers" firestore:"connectedInfluencers"`

	Credits BrandCredits `json:"credits" firestore:"credits"`

	HasPayWall bool `json:"hasPayWall" firestore:"hasPayWall"`

	// Members       []BrandMember  `json:"members" firestore:"members"`
	// Notifications []Notification `json:"notifications" firestore:"notifications"`
}
type BrandInfluencerConnect struct {
	Requested []string `json:"requested,omitempty" firestore:"requested,omitempty"`
	Connected []string `json:"connected,omitempty" firestore:"connected,omitempty"`
}
type BrandCredits struct {
	Influencer    int `json:"influencer" firestore:"influencer"`
	Discovery     int `json:"discovery" firestore:"discovery"`
	Connection    int `json:"connection" firestore:"connection"`
	Collaboration int `json:"collaboration" firestore:"collaboration"`
	Contract      int `json:"contract" firestore:"contract"`
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

var (
	PlanCreditsMap = map[string]BrandCredits{
		"starter": BrandCredits{
			Influencer:    5,
			Discovery:     1,
			Connection:    0,
			Collaboration: 1,
			Contract:      1,
		},
		"growth": BrandCredits{
			Influencer:    50,
			Discovery:     50,
			Connection:    10,
			Collaboration: 5,
			Contract:      8,
		},
		"pro": BrandCredits{
			Influencer:    200,
			Discovery:     100,
			Connection:    20,
			Collaboration: 100,
			Contract:      200,
		},
		"enterprise": BrandCredits{
			Influencer:    200,
			Discovery:     200,
			Connection:    50,
			Collaboration: 100,
			Contract:      200,
		},
	}
)

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
