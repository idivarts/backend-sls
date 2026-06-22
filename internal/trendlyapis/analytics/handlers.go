package analytics

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/internal/socialsync"
)

// GetBrandAnalyticsOverview queues a background refresh of the brand's unified
// analytics (the heavy all-accounts fan-out) and returns immediately. The worker
// writes the result to Firestore (brands/{brandId}/analyticsOverview/{range});
// the dashboard observes it live via its Firestore listener.
// GET /api/v2/brands/:brandId/analytics/overview?range=28d
func GetBrandAnalyticsOverview(c *gin.Context) {
	brandID := c.Param("brandId")
	if brandID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "brandId is required"})
		return
	}
	r := ParseRange(c.Query("range"))

	queued, err := socialsync.Enqueue(socialsync.Message{
		Type:    socialsync.OpAnalytics,
		BrandID: brandID,
		Range:   string(r),
	})
	if err != nil {
		log.Printf("analytics overview: enqueue failed for %s: %v", brandID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to queue analytics refresh"})
		return
	}
	if !queued {
		// No queue configured (local dev) — build inline; it still writes to
		// Firestore so the listener behaves identically, just synchronously.
		if err := Refresh(brandID, string(r), ""); err != nil {
			log.Printf("analytics overview: inline refresh failed for %s: %v", brandID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to refresh analytics"})
			return
		}
	}
	c.JSON(http.StatusAccepted, gin.H{"queued": true})
}

// Refresh rebuilds the analytics overview for a (brand, range) and persists it to
// Firestore. When socialID is set it recomputes ONLY that account and splices it
// into the existing overview doc (a unit-level "resync this page"); otherwise it
// rebuilds every account. Runs in the social_sqs worker off the request path.
func Refresh(brandID, rng, socialID string) error {
	r := ParseRange(rng)
	if socialID != "" {
		return refreshAccount(brandID, r, socialID)
	}

	accounts, err := trendlymodels.ListBrandSocialAccounts(brandID)
	if err != nil {
		return err
	}

	// Fan out per-account fetches concurrently; each is independently isolated
	// (errors surface on the account, never fail the whole dashboard).
	results := make([]AccountAnalytics, len(accounts))
	var wg sync.WaitGroup
	for i := range accounts {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = getAccountAnalytics(brandID, accounts[idx], r)
		}(i)
	}
	wg.Wait()

	return writeOverview(brandID, r, results)
}

// refreshAccount recomputes a single account and splices it into the existing
// overview doc (preserving the other accounts), then recomputes totals.
func refreshAccount(brandID string, r Range, socialID string) error {
	acc, err := trendlymodels.GetBrandSocialAccount(brandID, socialID)
	if err != nil {
		return err
	}
	updated := getAccountAnalytics(brandID, *acc, r)

	var accounts []AccountAnalytics
	if existing, err := trendlymodels.GetAnalyticsOverview(brandID, string(r)); err == nil && existing != nil && existing.Payload != "" {
		var ov Overview
		if err := json.Unmarshal([]byte(existing.Payload), &ov); err == nil {
			accounts = ov.Accounts
		}
	}
	replaced := false
	for i := range accounts {
		if accounts[i].SocialID == socialID {
			accounts[i] = updated
			replaced = true
			break
		}
	}
	if !replaced {
		accounts = append(accounts, updated)
	}
	return writeOverview(brandID, r, accounts)
}

// writeOverview computes totals over the accounts and persists the overview doc.
func writeOverview(brandID string, r Range, accounts []AccountAnalytics) error {
	overview := Overview{
		BrandID:     brandID,
		Range:       string(r),
		GeneratedAt: time.Now().Unix(),
		Totals:      computeTotals(accounts),
		Accounts:    accounts,
	}
	payload, err := json.Marshal(overview)
	if err != nil {
		return err
	}
	return trendlymodels.SetAnalyticsOverview(brandID, &trendlymodels.AnalyticsOverviewDoc{
		Range:       string(r),
		Payload:     string(payload),
		GeneratedAt: overview.GeneratedAt,
	})
}

// computeTotals sums followers + per-metric totals across accounts.
func computeTotals(accounts []AccountAnalytics) map[string]int64 {
	totals := map[string]int64{
		"followers":       0,
		BucketReach:       0,
		BucketImpressions: 0,
		BucketEngagement:  0,
		BucketViews:       0,
	}
	for _, a := range accounts {
		totals["followers"] += a.FollowerCount
		for key, m := range a.Metrics {
			totals[key] += m.Total
		}
	}
	return totals
}

// ResyncBrandAccountAnalytics queues a recompute of ONE account's analytics and
// returns immediately; the worker splices it into the overview doc and the
// dashboard updates live via its Firestore listener.
// POST /api/v2/brands/:brandId/analytics/accounts/:id/resync?range=28d
func ResyncBrandAccountAnalytics(c *gin.Context) {
	brandID := c.Param("brandId")
	socialID := c.Param("id")
	if brandID == "" || socialID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "brandId and account id are required"})
		return
	}
	r := ParseRange(c.Query("range"))

	queued, err := socialsync.Enqueue(socialsync.Message{
		Type:     socialsync.OpAnalytics,
		BrandID:  brandID,
		Range:    string(r),
		SocialID: socialID,
	})
	if err != nil {
		log.Printf("analytics account resync: enqueue failed for %s/%s: %v", brandID, socialID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to queue analytics resync"})
		return
	}
	if !queued {
		if err := Refresh(brandID, string(r), socialID); err != nil {
			log.Printf("analytics account resync: inline failed for %s/%s: %v", brandID, socialID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resync analytics"})
			return
		}
	}
	c.JSON(http.StatusAccepted, gin.H{"queued": true})
}

// GetBrandAccountAnalytics returns analytics for a single connected account.
// GET /api/v2/brands/:brandId/analytics/accounts/:id?range=28d
func GetBrandAccountAnalytics(c *gin.Context) {
	brandID := c.Param("brandId")
	socialID := c.Param("id")
	if brandID == "" || socialID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "brandId and account id are required"})
		return
	}
	r := ParseRange(c.Query("range"))

	acc, err := trendlymodels.GetBrandSocialAccount(brandID, socialID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "connected account not found"})
		return
	}

	c.JSON(http.StatusOK, getAccountAnalytics(brandID, *acc, r))
}
