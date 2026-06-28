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

// ─── AnalyticsOverview (brands/{brandId}/analyticsOverview/{range}) ───────────
//
// The latest computed analytics overview for a (brand, range), produced
// asynchronously by the social_sqs worker (OpAnalytics) so the dashboard reads
// it live from Firestore instead of blocking on a slow all-accounts Graph fetch.
// Payload is the JSON-encoded analytics.Overview (kept opaque here so this model
// doesn't depend on the analytics package). Written server-side only.

type AnalyticsOverviewDoc struct {
	Range       string `json:"range" firestore:"range"`
	Payload     string `json:"payload" firestore:"payload"` // JSON-encoded analytics.Overview
	GeneratedAt int64  `json:"generatedAt" firestore:"generatedAt"`
}

// GetAnalyticsOverview reads the overview doc for a (brand, range); returns
// (nil, nil) when absent (callers treat it as "no cached overview yet").
func GetAnalyticsOverview(brandID, rng string) (*AnalyticsOverviewDoc, error) {
	doc, err := firestoredb.Client.
		Collection(fmt.Sprintf("brands/%s/analyticsOverview", brandID)).
		Doc(rng).
		Get(context.Background())
	if err != nil {
		return nil, nil
	}
	var d AnalyticsOverviewDoc
	if err := doc.DataTo(&d); err != nil {
		return nil, err
	}
	return &d, nil
}

// SetAnalyticsOverview writes/overwrites the overview doc for a (brand, range).
func SetAnalyticsOverview(brandID string, d *AnalyticsOverviewDoc) error {
	_, err := firestoredb.Client.
		Collection(fmt.Sprintf("brands/%s/analyticsOverview", brandID)).
		Doc(d.Range).
		Set(context.Background(), d)
	return err
}

// ListAnalyticsOverviews returns every overview doc (one per range) for a brand.
// Used on disconnect to splice a removed account out of each cached overview.
func ListAnalyticsOverviews(brandID string) ([]AnalyticsOverviewDoc, error) {
	docs, err := firestoredb.Client.
		Collection(fmt.Sprintf("brands/%s/analyticsOverview", brandID)).
		Documents(context.Background()).
		GetAll()
	if err != nil {
		return nil, err
	}
	out := make([]AnalyticsOverviewDoc, 0, len(docs))
	for _, doc := range docs {
		var d AnalyticsOverviewDoc
		if err := doc.DataTo(&d); err != nil {
			return nil, fmt.Errorf("ListAnalyticsOverviews: decode %s: %w", doc.Ref.ID, err)
		}
		out = append(out, d)
	}
	return out, nil
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

// DeleteAnalyticsBySocial removes all cached analytics and daily snapshots for a
// connected account (used on disconnect). The per-range analyticsOverview docs
// still hold an entry for this account inside their JSON payload — those are
// spliced out separately by the analytics package on disconnect, since the
// payload is opaque to this model. Returns the total docs deleted.
func DeleteAnalyticsBySocial(brandID, socialID string) (int, error) {
	cache, err := deleteDocsBySocial(fmt.Sprintf("brands/%s/analyticsCache", brandID), socialID)
	if err != nil {
		return cache, err
	}
	snaps, err := deleteDocsBySocial(fmt.Sprintf("brands/%s/analyticsSnapshots", brandID), socialID)
	return cache + snaps, err
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
