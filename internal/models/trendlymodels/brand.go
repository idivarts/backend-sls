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

	// Members       []BrandMember  `json:"members" firestore:"members"`
	// Notifications []Notification `json:"notifications" firestore:"notifications"`
}
type BrandBilling struct {
	Subscription  *string `json:"subscription,omitempty" firestore:"subscription,omitempty"`
	BillingStatus *string `json:"billingStatus,omitempty" firestore:"billingStatus,omitempty"`
	IsGrowthPlan  *bool   `json:"isGrowthPlan,omitempty" firestore:"isGrowthPlan,omitempty"`
	IsOnTrial     *bool   `json:"isOnTrial,omitempty" firestore:"isOnTrial,omitempty"`
	Status        *int    `json:"status,omitempty" firestore:"status,omitempty"`
}

type BrandProfile struct {
	About      *string  `json:"about,omitempty" firestore:"about,omitempty"`
	Banner     *string  `json:"banner,omitempty" firestore:"banner,omitempty"`
	Industries []string `json:"industries,omitempty" firestore:"industries,omitempty"`
	Website    *string  `json:"website,omitempty" firestore:"website,omitempty"`
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
