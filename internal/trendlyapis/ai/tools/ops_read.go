package tools

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

func publishFailures() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_publish_failures",
			"List content whose latest publish failed or partially failed — candidates for a retry. Includes the error and per-destination results.",
			openrouter.ObjectSchema(map[string]any{}, nil),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			var items []trendlymodels.Content
			for _, st := range []string{"failed", "partially_failed"} {
				list, err := trendlymodels.ListContentByStatus(ctx, brandID, st)
				if err != nil {
					return nil, err
				}
				items = append(items, list...)
			}
			out := make([]map[string]any, 0, len(items))
			for _, ct := range items {
				out = append(out, map[string]any{
					"id": ct.ID, "title": ct.Title, "status": ct.Status,
					"publishError": ct.PublishError, "publishResults": ct.PublishResults,
					"postingTimeStamp": ct.PostingTimeStamp,
				})
			}
			return map[string]any{"count": len(out), "failures": out}, nil
		},
	}
}

func scheduleConflicts() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_schedule_conflicts",
			"Find content scheduled into the same platform + hour slot within a window — clashes to space out.",
			openrouter.ObjectSchema(dateRangeProps(nil), nil),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			start, end := rangeFromArgs(args)
			items, err := trendlymodels.ListContentInRange(ctx, brandID, start, end, false)
			if err != nil {
				return nil, err
			}
			// slot key = platform + "YYYY-MM-DD HH" -> content ids
			byslot := map[string][]string{}
			slotWhen := map[string]string{}
			for _, ct := range items {
				if ct.PostingTimeStamp <= 0 {
					continue
				}
				when := time.UnixMilli(ct.PostingTimeStamp).UTC().Format("2006-01-02 15h")
				for _, p := range ct.Platforms {
					key := p + " " + when
					byslot[key] = append(byslot[key], ct.ID)
					slotWhen[key] = when
				}
			}
			conflicts := make([]map[string]any, 0)
			for key, ids := range byslot {
				if len(ids) > 1 {
					conflicts = append(conflicts, map[string]any{"slot": key, "contentIds": ids, "count": len(ids)})
				}
			}
			sort.Slice(conflicts, func(i, j int) bool { return conflicts[i]["count"].(int) > conflicts[j]["count"].(int) })
			return map[string]any{"conflicts": conflicts, "conflictCount": len(conflicts)}, nil
		},
	}
}

func suggestOptimalSlot() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"suggest_optimal_slot",
			"Suggest upcoming open posting slots: empty days in the next N days paired with the brand's best-performing hour (from historical top media, falling back to 18:00 UTC).",
			openrouter.ObjectSchema(map[string]any{
				"days":  openrouter.NumberProp("How many upcoming days to consider (default 14)."),
				"limit": openrouter.NumberProp("Max slots to suggest (default 5)."),
			}, nil),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			days := argInt(args, "days", 14)
			limit := argInt(args, "limit", 5)
			now := time.Now().UTC()
			start := now.UnixMilli()
			end := now.AddDate(0, 0, days).UnixMilli()
			items, err := trendlymodels.ListContentInRange(ctx, brandID, start, end, false)
			if err != nil {
				return nil, err
			}
			have := map[string]bool{}
			for _, ct := range items {
				if ct.PostingTimeStamp > 0 {
					have[time.UnixMilli(ct.PostingTimeStamp).UTC().Format("2006-01-02")] = true
				}
			}
			// Best hour from top media (best-effort); default 18:00 UTC.
			bestHour := 18
			if ov, err := loadOverview(brandID, "90d"); err == nil {
				type agg struct{ posts, total int64 }
				byHour := map[int]*agg{}
				for _, m := range allTopMedia(ov) {
					if m.Timestamp <= 0 {
						continue
					}
					h := time.Unix(m.Timestamp, 0).UTC().Hour()
					a := byHour[h]
					if a == nil {
						a = &agg{}
						byHour[h] = a
					}
					a.posts++
					a.total += m.Engagement
				}
				var bestAvg int64 = -1
				for h, a := range byHour {
					avg := int64(0)
					if a.posts > 0 {
						avg = a.total / a.posts
					}
					if avg > bestAvg {
						bestAvg = avg
						bestHour = h
					}
				}
			}
			slots := make([]map[string]any, 0, limit)
			for i := 0; i < days && len(slots) < limit; i++ {
				d := now.AddDate(0, 0, i)
				key := d.Format("2006-01-02")
				if have[key] {
					continue
				}
				slots = append(slots, map[string]any{
					"date": key, "suggestedTime": fmt.Sprintf("%02d:00", bestHour), "timezone": "UTC",
				})
			}
			return map[string]any{"suggestedSlots": slots, "bestHourUTC": bestHour}, nil
		},
	}
}
