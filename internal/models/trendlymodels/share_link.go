package trendlymodels

import (
	"context"

	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

// ShareLink mirrors a top-level shareLinks/{token} document written by the brand
// app to expose a view-only resource (e.g. a calendar month) over a public link.
// The unauthenticated public endpoint resolves the token to this record.

const shareLinksCollection = "shareLinks"

type ShareLink struct {
	Type       string `json:"type" firestore:"type"`
	BrandID    string `json:"brandId" firestore:"brandId"`
	ResourceID string `json:"resourceId,omitempty" firestore:"resourceId"`
	Month      string `json:"month,omitempty" firestore:"month"`
	Enabled    bool   `json:"enabled" firestore:"enabled"`
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
