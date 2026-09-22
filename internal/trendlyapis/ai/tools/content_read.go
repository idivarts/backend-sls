package tools

import (
	"context"
	"sort"
	"time"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// summarizeContent produces a compact, token-cheap view of a content item for
// list results (full detail is available via get_content_details).
func summarizeContent(ct trendlymodels.Content) map[string]any {
	return map[string]any{
		"id":               ct.ID,
		"title":            ct.Title,
		"status":           ct.Status,
		"format":           ct.ContentFormat,
		"platforms":        ct.Platforms,
		"pillars":          ct.ContentPillars,
		"postingTimeStamp": ct.PostingTimeStamp,
	}
}

// dateRangeProps are the shared month/year or startMs/endMs window params.
func dateRangeProps(extra map[string]any) map[string]any {
	base := map[string]any{
		"month":   openrouter.NumberProp("Month number 1-12 (defaults to current month). Ignored if startMs is given."),
		"year":    openrouter.NumberProp("Four-digit year (defaults to current year). Ignored if startMs is given."),
		"startMs": openrouter.NumberProp("Optional explicit window start (epoch ms). Overrides month/year."),
		"endMs":   openrouter.NumberProp("Optional explicit window end (epoch ms)."),
	}
	for k, v := range extra {
		base[k] = v
	}
	return base
}

func contentInTimeframe() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_content_in_timeframe",
			"List the brand's planned/scheduled/published content within a date window, optionally filtered by status. Returns compact summaries; use get_content_details for one post's full body.",
			openrouter.ObjectSchema(dateRangeProps(map[string]any{
				"status": openrouter.StringProp("Optional status filter (e.g. draft, scheduled, published, failed)."),
			}), nil),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			start, end := rangeFromArgs(args)
			status := argStr(args, "status")
			items, err := trendlymodels.ListContentInRange(ctx, brandID, start, end, true)
			if err != nil {
				return nil, err
			}
			out := make([]map[string]any, 0, len(items))
			for _, ct := range items {
				if status != "" && ct.Status != status {
					continue
				}
				out = append(out, summarizeContent(ct))
			}
			return map[string]any{"count": len(out), "content": out}, nil
		},
	}
}

func contentDetails() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_content_details",
			"Fetch the full body of a single content item: caption, hashtags, script, description, attachments, pillars, publish results and posted URL.",
			openrouter.ObjectSchema(map[string]any{
				"contentId": openrouter.StringProp("The content document ID."),
			}, []string{"contentId"}),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			id := argStr(args, "contentId")
			if id == "" {
				return map[string]any{"error": "contentId is required"}, nil
			}
			ct, err := trendlymodels.GetContent(brandID, id)
			if err != nil {
				return nil, err
			}
			atts := make([]map[string]any, 0, len(ct.Attachments))
			for _, a := range ct.Attachments {
				atts = append(atts, map[string]any{"type": a.Type, "imageUrl": a.ImageURL, "playUrl": a.PlayURL})
			}
			return map[string]any{
				"id":               ct.ID,
				"title":            ct.Title,
				"caption":          ct.Caption,
				"hashtags":         ct.Hashtags,
				"script":           ct.Script,
				"description":      ct.Description,
				"status":           ct.Status,
				"format":           ct.ContentFormat,
				"platforms":        ct.Platforms,
				"pillars":          ct.ContentPillars,
				"postingTimeStamp": ct.PostingTimeStamp,
				"attachments":      atts,
				"publishResults":   ct.PublishResults,
				"postedUrl":        ct.PostedURL,
			}, nil
		},
	}
}

func contentGaps() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_content_gaps",
			"Find days within a window that have NO planned content — useful for spotting holes in the posting cadence before planning.",
			openrouter.ObjectSchema(dateRangeProps(nil), nil),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			start, end := rangeFromArgs(args)
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
			gaps := []string{}
			for t := time.UnixMilli(start).UTC(); t.Before(time.UnixMilli(end).UTC()); t = t.AddDate(0, 0, 1) {
				d := t.Format("2006-01-02")
				if !have[d] {
					gaps = append(gaps, d)
				}
			}
			return map[string]any{
				"emptyDays":       gaps,
				"emptyDayCount":   len(gaps),
				"plannedDayCount": len(have),
			}, nil
		},
	}
}

func draftBacklog() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_draft_backlog",
			"List the brand's draft (unscheduled) content waiting to be planned or published.",
			openrouter.ObjectSchema(map[string]any{
				"limit": openrouter.NumberProp("Maximum drafts to return (default 50)."),
			}, nil),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			items, err := trendlymodels.ListContentByStatus(ctx, brandID, "draft")
			if err != nil {
				return nil, err
			}
			limit := argInt(args, "limit", 50)
			out := make([]map[string]any, 0, len(items))
			for _, ct := range items {
				if len(out) >= limit {
					break
				}
				out = append(out, summarizeContent(ct))
			}
			return map[string]any{"count": len(out), "drafts": out}, nil
		},
	}
}

// pillarCounts tallies ContentPillars usage across content in a window.
func pillarCounts(ctx context.Context, brandID string, start, end int64) ([]map[string]any, int, error) {
	items, err := trendlymodels.ListContentInRange(ctx, brandID, start, end, true)
	if err != nil {
		return nil, 0, err
	}
	counts := map[string]int{}
	for _, ct := range items {
		for _, p := range ct.ContentPillars {
			if p != "" {
				counts[p]++
			}
		}
	}
	out := make([]map[string]any, 0, len(counts))
	for p, n := range counts {
		out = append(out, map[string]any{"pillar": p, "uses": n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["uses"].(int) > out[j]["uses"].(int) })
	return out, len(items), nil
}

func contentPillars() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_content_pillars",
			"List the distinct content pillars/themes the brand actually uses (derived from tagged content over the last ~12 months), with usage counts.",
			openrouter.ObjectSchema(map[string]any{}, nil),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			now := time.Now().UTC()
			start := now.AddDate(-1, 0, 0).UnixMilli()
			end := now.AddDate(0, 3, 0).UnixMilli()
			pillars, scanned, err := pillarCounts(ctx, brandID, start, end)
			if err != nil {
				return nil, err
			}
			return map[string]any{"pillars": pillars, "distinctCount": len(pillars), "contentScanned": scanned}, nil
		},
	}
}

func contentPillarBalance() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_content_pillar_balance",
			"Show how many posts fall under each content pillar within a date window — surfaces over- and under-served themes for planning.",
			openrouter.ObjectSchema(dateRangeProps(nil), nil),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			start, end := rangeFromArgs(args)
			pillars, scanned, err := pillarCounts(ctx, brandID, start, end)
			if err != nil {
				return nil, err
			}
			return map[string]any{"balance": pillars, "contentInRange": scanned}, nil
		},
	}
}
