package analytics

import (
	"log"
	"time"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

// lastSeriesPoint returns the most recent daily value of a metric's series,
// or 0 when the metric has no time series (e.g. IG engagement totals).
func lastSeriesPoint(m Metric) int64 {
	if n := len(m.Series); n > 0 {
		return m.Series[n-1].Value
	}
	return 0
}

// SnapshotBrand fetches fresh analytics for each Meta account of a brand and
// writes one daily top-line snapshot per account. It is called by the snapshot
// cron. `date` is the UTC YYYY-MM-DD the snapshot is filed under.
//
// Followers are stored point-in-time (a clean daily trend). Reach/views/
// engagement are stored as the latest daily series point where available, so
// accumulated snapshots form a per-day trend the live API can't reproduce.
func SnapshotBrand(brandID, date string) (written, failed int) {
	accounts, err := trendlymodels.ListBrandSocialAccounts(brandID)
	if err != nil {
		log.Printf("snapshot: list socials failed for %s: %v", brandID, err)
		return 0, 0
	}

	now := time.Now().Unix()
	for _, acc := range accounts {
		if acc.Platform != trendlymodels.PlatformInstagram &&
			acc.Platform != trendlymodels.PlatformFacebook {
			continue
		}

		a := fetchMetaAccount(brandID, acc, Range7d)
		if a.Error != "" {
			log.Printf("snapshot: fetch failed %s/%s: %s", brandID, acc.ID, a.Error)
			failed++
			continue
		}

		snap := &trendlymodels.AnalyticsSnapshot{
			SocialID:    acc.ID,
			Platform:    acc.Platform,
			Date:        date,
			Followers:   a.FollowerCount,
			Reach:       lastSeriesPoint(a.Metrics[BucketReach]),
			Impressions: lastSeriesPoint(a.Metrics[BucketImpressions]),
			Engagement:  lastSeriesPoint(a.Metrics[BucketEngagement]),
			Views:       lastSeriesPoint(a.Metrics[BucketViews]),
			CreatedAt:   now,
		}
		if err := trendlymodels.SetAnalyticsSnapshot(brandID, snap); err != nil {
			log.Printf("snapshot: write failed %s/%s: %v", brandID, acc.ID, err)
			failed++
			continue
		}
		written++
	}
	return written, failed
}
