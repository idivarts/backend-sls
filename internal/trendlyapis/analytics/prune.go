package analytics

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

// PruneAccountFromOverviews removes a disconnected social's AccountAnalytics
// entry from every cached overview doc for the brand (one per range) and
// recomputes totals so the dashboard stops showing the dead account immediately,
// instead of waiting for the next full Refresh. Best-effort: writes that fail
// are reported via the returned error but later ranges are still processed.
func PruneAccountFromOverviews(brandID, socialID string) error {
	if brandID == "" || socialID == "" {
		return fmt.Errorf("PruneAccountFromOverviews: empty brandID or socialID")
	}
	docs, err := trendlymodels.ListAnalyticsOverviews(brandID)
	if err != nil {
		return err
	}
	var firstErr error
	for _, d := range docs {
		if d.Payload == "" {
			continue
		}
		var ov Overview
		if err := json.Unmarshal([]byte(d.Payload), &ov); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("decode overview %s: %w", d.Range, err)
			}
			continue
		}
		filtered := make([]AccountAnalytics, 0, len(ov.Accounts))
		removed := false
		for _, a := range ov.Accounts {
			if a.SocialID == socialID {
				removed = true
				continue
			}
			filtered = append(filtered, a)
		}
		if !removed {
			continue
		}
		ov.Accounts = filtered
		ov.Totals = computeTotals(filtered)
		ov.GeneratedAt = time.Now().Unix()
		payload, err := json.Marshal(ov)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("encode overview %s: %w", d.Range, err)
			}
			continue
		}
		if err := trendlymodels.SetAnalyticsOverview(brandID, &trendlymodels.AnalyticsOverviewDoc{
			Range:       d.Range,
			Payload:     string(payload),
			GeneratedAt: ov.GeneratedAt,
		}); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("write overview %s: %w", d.Range, err)
			}
		}
	}
	return firstErr
}
