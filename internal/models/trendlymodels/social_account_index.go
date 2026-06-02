package trendlymodels

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

// ─── SocialAccountIndex (socialAccountIndex/{platformAccountId}) ───────────────
//
// Reverse lookup from a platform account id (Instagram Business Account id or
// Facebook Page id) to the Trendly entity that connected it. Meta webhooks are
// delivered keyed by this platform account id (entry.id), so the inbox webhook
// router reads this top-level collection to resolve an incoming event in O(1)
// to the brand/user + social account whose token can serve it.
//
// Doc id = the platform account id itself. Note: if the same platform account
// is connected by more than one Trendly entity, this is last-write-wins for v1.

const socialAccountIndexCollection = "socialAccountIndex"

type SocialAccountIndex struct {
	// PlatformAccountID is also the document id.
	PlatformAccountID string   `json:"platformAccountId" firestore:"platformAccountId"`
	Platform          Platform `json:"platform" firestore:"platform"`

	// App is "brands" or "users" — tells the router which store to write to.
	App string `json:"app" firestore:"app"`

	// Exactly one of BrandID / UserID is the owning scope (BrandID when App=="brands").
	BrandID string `json:"brandId,omitempty" firestore:"brandId,omitempty"`
	UserID  string `json:"userId,omitempty" firestore:"userId,omitempty"`

	// SocialID is the SocialAccount.ID (hash) whose token serves this account.
	SocialID string `json:"socialId" firestore:"socialId"`

	UpdatedAt int64 `json:"updatedAt" firestore:"updatedAt"`
}

// Set creates or overwrites the index document keyed by PlatformAccountID.
func (idx *SocialAccountIndex) Set() (*firestore.WriteResult, error) {
	if idx.PlatformAccountID == "" {
		return nil, fmt.Errorf("SocialAccountIndex.Set: empty PlatformAccountID")
	}
	return firestoredb.Client.
		Collection(socialAccountIndexCollection).
		Doc(idx.PlatformAccountID).
		Set(context.Background(), idx)
}

// GetSocialAccountIndex resolves a platform account id (webhook entry.id) to the
// connected account that can serve it.
func GetSocialAccountIndex(platformAccountID string) (*SocialAccountIndex, error) {
	doc, err := firestoredb.Client.
		Collection(socialAccountIndexCollection).
		Doc(platformAccountID).
		Get(context.Background())
	if err != nil {
		return nil, err
	}
	var idx SocialAccountIndex
	if err := doc.DataTo(&idx); err != nil {
		return nil, err
	}
	return &idx, nil
}

// DeleteSocialAccountIndex removes the index document for a platform account id.
// Safe to call for ids that may not exist.
func DeleteSocialAccountIndex(platformAccountID string) error {
	if platformAccountID == "" {
		return nil
	}
	_, err := firestoredb.Client.
		Collection(socialAccountIndexCollection).
		Doc(platformAccountID).
		Delete(context.Background())
	return err
}
