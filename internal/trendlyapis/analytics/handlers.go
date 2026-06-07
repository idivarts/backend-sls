package analytics

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

// GetBrandAnalyticsOverview returns unified analytics across all of a brand's
// connected accounts.
// GET /api/v2/brands/:brandId/analytics/overview?range=28d
func GetBrandAnalyticsOverview(c *gin.Context) {
	brandID := c.Param("brandId")
	if brandID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "brandId is required"})
		return
	}
	r := ParseRange(c.Query("range"))

	accounts, err := trendlymodels.ListBrandSocialAccounts(brandID)
	if err != nil {
		log.Printf("analytics overview: list socials failed for %s: %v", brandID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch connected accounts"})
		return
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

	totals := map[string]int64{
		"followers":   0,
		BucketReach:   0,
		BucketImpressions: 0,
		BucketEngagement:  0,
		BucketViews:       0,
	}
	for _, a := range results {
		totals["followers"] += a.FollowerCount
		for key, m := range a.Metrics {
			totals[key] += m.Total
		}
	}

	c.JSON(http.StatusOK, Overview{
		BrandID:     brandID,
		Range:       string(r),
		GeneratedAt: time.Now().Unix(),
		Totals:      totals,
		Accounts:    results,
	})
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
