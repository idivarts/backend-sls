package trendlymodels

import (
	"context"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

// ShareLink mirrors a top-level shareLinks/{token} document written by the brand
// app to expose a view-only resource (e.g. a calendar month) over a public link.
// The unauthenticated public endpoint resolves the token to this record.

const shareLinksCollection = "shareLinks"

// Share types, mirroring ShareType in
// shared-libs/firestore/trendly-pro/models/share-links.ts.
const (
	ShareTypeStrategy      = "strategy"
	ShareTypeCalendarMonth = "calendarMonth"
	ShareTypeContent       = "content"
)

type ShareLink struct {
	Type       string `json:"type" firestore:"type"`
	BrandID    string `json:"brandId" firestore:"brandId"`
	ResourceID string `json:"resourceId,omitempty" firestore:"resourceId"`
	Month      string `json:"month,omitempty" firestore:"month"`
	Enabled    bool   `json:"enabled" firestore:"enabled"`

	// DeepLink is the Branch link minted for this token (pkg/branch). Written
	// server-side only and cached here so re-opening the share sheet reuses the
	// same URL instead of minting a new one on every open — a Branch link is
	// permanent, and churning them would fragment its click analytics.
	DeepLink string `json:"deepLink,omitempty" firestore:"deepLink"`
	// DeepLinkCreatedAt is epoch ms, matching the app's other timestamps.
	DeepLinkCreatedAt int64 `json:"deepLinkCreatedAt,omitempty" firestore:"deepLinkCreatedAt"`
	// DeepLinkPreview fingerprints the unfurl card the link was minted with, so a
	// resource that has since been renamed (or has gained an image) is detected
	// and the existing link re-pointed, rather than left showing a stale card.
	DeepLinkPreview string `json:"deepLinkPreview,omitempty" firestore:"deepLinkPreview"`
}

// GetShareLink resolves a share-link token to its record.
func GetShareLink(ctx context.Context, token string) (*ShareLink, error) {
	doc, err := firestoredb.Client.Collection(shareLinksCollection).Doc(token).Get(ctx)
	if err != nil {
		return nil, err
	}
	var link ShareLink
	if err := doc.DataTo(&link); err != nil {
		return nil, err
	}
	return &link, nil
}

// SetShareLinkDeepLink caches the minted Branch link on the share-link doc.
// Merged rather than set, so it cannot clobber the fields the app owns
// (enabled/type/resourceId/…).
func SetShareLinkDeepLink(ctx context.Context, token, deepLink, previewFingerprint string, createdAt int64) error {
	_, err := firestoredb.Client.Collection(shareLinksCollection).Doc(token).Set(ctx, map[string]interface{}{
		"deepLink":          deepLink,
		"deepLinkCreatedAt": createdAt,
		"deepLinkPreview":   previewFingerprint,
	}, firestore.MergeAll)
	return err
}

// SetShareLinkPreview records the card an existing deep link now carries, after
// the link itself has been re-pointed at Branch.
func SetShareLinkPreview(ctx context.Context, token, previewFingerprint string) error {
	_, err := firestoredb.Client.Collection(shareLinksCollection).Doc(token).Set(ctx, map[string]interface{}{
		"deepLinkPreview": previewFingerprint,
	}, firestore.MergeAll)
	return err
}
