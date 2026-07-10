package tools

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	analytics "github.com/idivarts/backend-sls/internal/trendlyapis/analytics"
)

// ─── argument helpers ───────────────────────────────────────────────────────
// Tool arguments arrive as a decoded JSON object (map[string]any); numbers are
// always float64. These helpers read values defensively with sane defaults.

func argStr(args map[string]any, key string) string {
	if s, ok := args[key].(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func argInt(args map[string]any, key string, def int) int {
	if v, ok := args[key].(float64); ok {
		return int(v)
	}
	return def
}

func argInt64(args map[string]any, key string, def int64) int64 {
	if v, ok := args[key].(float64); ok {
		return int64(v)
	}
	return def
}

func argBool(args map[string]any, key string) bool {
	b, _ := args[key].(bool)
	return b
}

// rangeFromArgs resolves a [start,end) millisecond window from either an explicit
// {startMs,endMs} pair or a {month,year} pair (defaulting to the current month).
func rangeFromArgs(args map[string]any) (int64, int64) {
	if s := argInt64(args, "startMs", 0); s > 0 {
		e := argInt64(args, "endMs", 0)
		if e <= 0 {
			e = time.Now().UnixMilli()
		}
		return s, e
	}
	now := time.Now().UTC()
	month := argInt(args, "month", int(now.Month()))
	year := argInt(args, "year", now.Year())
	if month < 1 || month > 12 {
		month = int(now.Month())
	}
	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	return start.UnixMilli(), end.UnixMilli()
}

// ─── analytics overview helpers ─────────────────────────────────────────────

// loadOverview returns the brand's cached analytics overview for a range. If no
// cached overview exists yet, it triggers one inline build (the same path the
// dashboard uses in no-queue mode) and re-reads. Ranges are "7d"|"28d"|"90d".
func loadOverview(brandID, rng string) (*analytics.Overview, error) {
	r := string(analytics.ParseRange(rng))
	if ov, ok := readOverviewDoc(brandID, r); ok {
		return ov, nil
	}
	if err := analytics.Refresh(brandID, r, ""); err != nil {
		return nil, err
	}
	if ov, ok := readOverviewDoc(brandID, r); ok {
		return ov, nil
	}
	return nil, errors.New("analytics not available yet")
}

func readOverviewDoc(brandID, r string) (*analytics.Overview, bool) {
	doc, err := trendlymodels.GetAnalyticsOverview(brandID, r)
	if err != nil || doc == nil || doc.Payload == "" {
		return nil, false
	}
	var ov analytics.Overview
	if json.Unmarshal([]byte(doc.Payload), &ov) != nil {
		return nil, false
	}
	return &ov, true
}

// allTopMedia flattens every account's top media into a single slice, tagging
// each with its owning platform/username for context.
type mediaRow struct {
	analytics.TopMedia
	Platform string `json:"platform"`
	Username string `json:"username"`
}

func allTopMedia(ov *analytics.Overview) []mediaRow {
	var out []mediaRow
	for _, acc := range ov.Accounts {
		for _, m := range acc.TopMedia {
			out = append(out, mediaRow{TopMedia: m, Platform: acc.Platform, Username: acc.Username})
		}
	}
	return out
}

// sortMediaByMetric sorts rows by the chosen metric (engagement|likes|comments),
// descending by default. Returns the same slice for convenience.
func sortMediaByMetric(rows []mediaRow, metric string, asc bool) []mediaRow {
	val := func(m mediaRow) int64 {
		switch metric {
		case "likes":
			return m.Likes
		case "comments":
			return m.Comments
		default:
			return m.Engagement
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if asc {
			return val(rows[i]) < val(rows[j])
		}
		return val(rows[i]) > val(rows[j])
	})
	return rows
}
