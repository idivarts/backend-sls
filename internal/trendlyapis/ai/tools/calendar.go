package tools

import (
	"context"
	"time"

	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
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
			iter := firestoredb.Client.
				Collection("brands").Doc(brandID).
				Collection("contents").
				Where("postingTimeStamp", ">=", start).
				Where("postingTimeStamp", "<", end).
				Documents(ctx)
			defer iter.Stop()

			var out []map[string]any
			for {
				doc, err := iter.Next()
				if err != nil {
					break
				}
				d := doc.Data()
				out = append(out, map[string]any{
					"id":               doc.Ref.ID,
					"title":            d["title"],
					"platform":         d["platform"],
					"contentFormat":    d["contentFormat"],
					"status":           d["status"],
					"postingTimeStamp": d["postingTimeStamp"],
				})
			}
			return out, nil
		},
	}
}
