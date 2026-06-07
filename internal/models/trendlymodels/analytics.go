package trendlymodels

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

// ─── AnalyticsCache (brands/{brandId}/analyticsCache/{socialId_range}) ─────────
//
// A short-TTL memo of a live Meta analytics fetch. The payload is the
// JSON-encoded per-account analytics blob (kept opaque here so this model
// doesn't depend on the analytics package). Server-side only.

type AnalyticsCacheDoc struct {
	SocialID  string `json:"socialId" firestore:"socialId"`
	Range     string `json:"range" firestore:"range"`
	Payload   string `json:"payload" firestore:"payload"` // JSON-encoded AccountAnalytics
	FetchedAt int64  `json:"fetchedAt" firestore:"fetchedAt"`
}

// AnalyticsCacheID is the deterministic doc id for a (social, range) pair.
func AnalyticsCacheID(socialID, rng string) string {
	return fmt.Sprintf("%s_%s", socialID, rng)
}

// GetAnalyticsCache reads a cache doc; returns (nil, nil) when absent.
func GetAnalyticsCache(brandID, socialID, rng string) (*AnalyticsCacheDoc, error) {
	doc, err := firestoredb.Client.
		Collection(fmt.Sprintf("brands/%s/analyticsCache", brandID)).
		Doc(AnalyticsCacheID(socialID, rng)).
		Get(context.Background())
	if err != nil {
		// Not found is not an error for callers — they treat it as a cache miss.
		return nil, nil
	}
	var d AnalyticsCacheDoc
	if err := doc.DataTo(&d); err != nil {
		return nil, err
	}
	return &d, nil
}

// SetAnalyticsCache writes/overwrites a cache doc.
func SetAnalyticsCache(brandID string, d *AnalyticsCacheDoc) error {
	_, err := firestoredb.Client.
		Collection(fmt.Sprintf("brands/%s/analyticsCache", brandID)).
		Doc(AnalyticsCacheID(d.SocialID, d.Range)).
		Set(context.Background(), d)
	return err
}

// ─── AnalyticsSnapshot (brands/{brandId}/analyticsSnapshots/{socialId_date}) ───
//
// One daily top-line snapshot per connected account, written by the snapshot
// cron. Powers historical trend graphs that the live Meta API can't serve.

type AnalyticsSnapshot struct {
	SocialID    string `json:"socialId" firestore:"socialId"`
	Platform    string `json:"platform" firestore:"platform"`
	Date        string `json:"date" firestore:"date"` // YYYY-MM-DD (UTC)
	Followers   int64  `json:"followers" firestore:"followers"`
	Reach       int64  `json:"reach" firestore:"reach"`
	Impressions int64  `json:"impressions" firestore:"impressions"`
	Engagement  int64  `json:"engagement" firestore:"engagement"`
	Views       int64  `json:"views" firestore:"views"`
	CreatedAt   int64  `json:"createdAt" firestore:"createdAt"`
}

// SnapshotID is the deterministic doc id for a (social, date) pair, so re-runs
// on the same day overwrite rather than duplicate.
func SnapshotID(socialID, date string) string {
	return fmt.Sprintf("%s_%s", socialID, date)
}

// SetAnalyticsSnapshot writes/overwrites a daily snapshot.
func SetAnalyticsSnapshot(brandID string, s *AnalyticsSnapshot) error {
	_, err := firestoredb.Client.
		Collection(fmt.Sprintf("brands/%s/analyticsSnapshots", brandID)).
		Doc(SnapshotID(s.SocialID, s.Date)).
		Set(context.Background(), s)
	return err
}

// ListAnalyticsSnapshots returns snapshots for one account on/after sinceDate,
// ordered by date ascending. Requires a composite index on (socialId, date).
func ListAnalyticsSnapshots(brandID, socialID, sinceDate string) ([]AnalyticsSnapshot, error) {
	docs, err := firestoredb.Client.
		Collection(fmt.Sprintf("brands/%s/analyticsSnapshots", brandID)).
		Where("socialId", "==", socialID).
		Where("date", ">=", sinceDate).
		OrderBy("date", firestore.Asc).
		Documents(context.Background()).
		GetAll()
	if err != nil {
		return nil, err
	}
	out := make([]AnalyticsSnapshot, 0, len(docs))
	for _, doc := range docs {
		var s AnalyticsSnapshot
		if err := doc.DataTo(&s); err != nil {
			return nil, fmt.Errorf("ListAnalyticsSnapshots: decode %s: %w", doc.Ref.ID, err)
		}
		out = append(out, s)
	}
	return out, nil
}
