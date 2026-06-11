package tools

import (
	"context"
	"time"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

func calendarPosts() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_calendar_posts",
			"Get scheduled content posts for the brand within a date range.",
			openrouter.ObjectSchema(map[string]any{
				"month": openrouter.NumberProp("Month number 1-12 (defaults to current month)"),
				"year":  openrouter.NumberProp("Four-digit year (defaults to current year)"),
			}, nil),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			now := time.Now()
			month := int(now.Month())
			year := now.Year()
			if v, ok := args["month"].(float64); ok && int(v) >= 1 && int(v) <= 12 {
				month = int(v)
			}
			if v, ok := args["year"].(float64); ok && int(v) > 2000 {
				year = int(v)
			}
			start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC).UnixMilli()
			end := time.Date(year, time.Month(month+1), 1, 0, 0, 0, 0, time.UTC).UnixMilli()
			contents, err := trendlymodels.ListContentInRange(ctx, brandID, start, end, true)
			if err != nil {
				return nil, err
			}

			var out []map[string]any
			for _, ct := range contents {
				out = append(out, map[string]any{
					"id":               ct.ID,
					"title":            ct.Title,
					"platform":         ct.Platform,
					"contentFormat":    ct.ContentFormat,
					"status":           ct.Status,
					"postingTimeStamp": ct.PostingTimeStamp,
				})
			}
			return out, nil
		},
	}
}
