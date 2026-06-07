package trendlymodels

import (
	"context"

	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"google.golang.org/api/iterator"
)

const AGENCY_HIRES_COLLECTION = "agency-hires"

// AgencyHire mirrors the subset of the `agency-hires` Firestore document
// (written by the brands app) that the backend needs to read.
type AgencyHire struct {
	BrandID   string `json:"brandId" firestore:"brandId"`
	ManagerID string `json:"managerId" firestore:"managerId"`
	Status    string `json:"status" firestore:"status"` // "draft" | "active" | "past"
}

// BrandHasActiveAgencyHire reports whether the given brand currently has at
// least one agency-hire with status "active". This is the server-side gate for
// allowing brands to invite discover-only (off-platform) influencers.
func BrandHasActiveAgencyHire(brandID string) (bool, error) {
	iter := firestoredb.Client.Collection(AGENCY_HIRES_COLLECTION).
		Where("brandId", "==", brandID).
		Where("status", "==", "active").
		Limit(1).
		Documents(context.Background())
	defer iter.Stop()

	_, err := iter.Next()
	if err == iterator.Done {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
