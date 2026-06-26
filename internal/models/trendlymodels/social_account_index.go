package trendlymodels

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ─── SocialAccountIndex (socialAccountIndex/{platformAccountId}) ───────────────
//
// Reverse lookup from a platform account id (Instagram Business Account id or
// Facebook Page id) to the Trendly entities that connected it. Meta webhooks are
// delivered keyed by this platform account id (entry.id), so the inbox webhook
// router reads this top-level collection to resolve an incoming event to every
// owner whose token can serve it.
//
// Doc id = the platform account id itself. The SAME account may be connected by
// more than one brand (e.g. an agency + the brand itself), so ownership is an
// ARRAY (Owners) and the webhook fans out to each owner's inbox. A single O(1)
// Get on the hot webhook path still resolves all owners.

const socialAccountIndexCollection = "socialAccountIndex"

// SocialIndexOwner is one Trendly entity that has connected a platform account.
type SocialIndexOwner struct {
	// App is "brands" or "users" — tells the router which store to write to.
	App string `json:"app" firestore:"app"`
	// Exactly one of BrandID / UserID is set (BrandID when App == "brands").
	BrandID string `json:"brandId,omitempty" firestore:"brandId,omitempty"`
	UserID  string `json:"userId,omitempty" firestore:"userId,omitempty"`
	// SocialID is this owner's SocialAccount.ID (hash) whose token serves the account.
	SocialID  string `json:"socialId" firestore:"socialId"`
	UpdatedAt int64  `json:"updatedAt" firestore:"updatedAt"`
}

// scopeID returns the owning entity id (brand or user) — the identity used to
// dedupe and remove owners.
func (o SocialIndexOwner) scopeID() string {
	if o.BrandID != "" {
		return o.BrandID
	}
	return o.UserID
}

type SocialAccountIndex struct {
	// PlatformAccountID is also the document id.
	PlatformAccountID string   `json:"platformAccountId" firestore:"platformAccountId"`
	Platform          Platform `json:"platform" firestore:"platform"`

	// Owners is the set of entities that connected this account.
	Owners []SocialIndexOwner `json:"owners,omitempty" firestore:"owners,omitempty"`

	// ── Legacy single-owner fields (pre multi-owner migration) ──
	// Older docs stored one owner inline. AllOwners() synthesizes an owner from
	// these when Owners is empty; AddSocialAccountOwner migrates them into Owners
	// and clears them on the next write.
	App     string `json:"app,omitempty" firestore:"app,omitempty"`
	BrandID string `json:"brandId,omitempty" firestore:"brandId,omitempty"`
	UserID  string `json:"userId,omitempty" firestore:"userId,omitempty"`
	SocialID string `json:"socialId,omitempty" firestore:"socialId,omitempty"`

	UpdatedAt int64 `json:"updatedAt" firestore:"updatedAt"`
}

// AllOwners returns the account's owners, synthesizing a single owner from the
// legacy inline fields for docs written before the multi-owner migration.
func (idx *SocialAccountIndex) AllOwners() []SocialIndexOwner {
	if len(idx.Owners) > 0 {
		return idx.Owners
	}
	if idx.App == "" && idx.BrandID == "" && idx.UserID == "" {
		return nil
	}
	return []SocialIndexOwner{{
		App:       idx.App,
		BrandID:   idx.BrandID,
		UserID:    idx.UserID,
		SocialID:  idx.SocialID,
		UpdatedAt: idx.UpdatedAt,
	}}
}

// GetSocialAccountIndex resolves a platform account id (webhook entry.id) to the
// connected accounts that can serve it.
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

// AddSocialAccountOwner upserts one owner into a platform account's index entry,
// creating the doc if needed. Idempotent per owner (deduped by App + scope id);
// re-connecting refreshes that owner's SocialID/UpdatedAt without disturbing the
// others. Runs in a transaction so concurrent connects can't drop an owner.
func AddSocialAccountOwner(platformAccountID string, platform Platform, owner SocialIndexOwner) error {
	if platformAccountID == "" {
		return fmt.Errorf("AddSocialAccountOwner: empty platformAccountID")
	}
	ref := firestoredb.Client.Collection(socialAccountIndexCollection).Doc(platformAccountID)
	return firestoredb.Client.RunTransaction(context.Background(), func(ctx context.Context, tx *firestore.Transaction) error {
		var idx SocialAccountIndex
		snap, err := tx.Get(ref)
		if err != nil && status.Code(err) != codes.NotFound {
			return err
		}
		if err == nil {
			if derr := snap.DataTo(&idx); derr != nil {
				return derr
			}
		}

		idx.PlatformAccountID = platformAccountID
		idx.Platform = platform
		idx.UpdatedAt = owner.UpdatedAt

		owners := idx.AllOwners()
		replaced := false
		for i := range owners {
			if owners[i].App == owner.App && owners[i].scopeID() == owner.scopeID() {
				owners[i] = owner
				replaced = true
				break
			}
		}
		if !replaced {
			owners = append(owners, owner)
		}
		idx.Owners = owners

		// Clear legacy inline owner fields — ownership now lives in Owners.
		idx.App, idx.BrandID, idx.UserID, idx.SocialID = "", "", "", ""

		return tx.Set(ref, &idx)
	})
}

// RemoveSocialAccountOwner removes one owner (matched by app + scope id) from a
// platform account's index entry. When the last owner is removed the doc is
// deleted. Other owners keep receiving webhooks. Safe for ids/owners that don't
// exist. Runs in a transaction to stay consistent with concurrent connects.
func RemoveSocialAccountOwner(platformAccountID, app, scopeID string) error {
	if platformAccountID == "" {
		return nil
	}
	ref := firestoredb.Client.Collection(socialAccountIndexCollection).Doc(platformAccountID)
	return firestoredb.Client.RunTransaction(context.Background(), func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(ref)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return nil // nothing to remove
			}
			return err
		}
		var idx SocialAccountIndex
		if derr := snap.DataTo(&idx); derr != nil {
			return derr
		}

		kept := make([]SocialIndexOwner, 0, len(idx.Owners))
		for _, o := range idx.AllOwners() {
			if o.App == app && o.scopeID() == scopeID {
				continue
			}
			kept = append(kept, o)
		}

		if len(kept) == 0 {
			return tx.Delete(ref)
		}

		idx.Owners = kept
		idx.App, idx.BrandID, idx.UserID, idx.SocialID = "", "", "", ""
		return tx.Set(ref, &idx)
	})
}

// DeleteSocialAccountIndex removes the entire index document for a platform
// account id (all owners). Prefer RemoveSocialAccountOwner for normal disconnects;
// this is for hard cleanup. Safe for ids that may not exist.
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
