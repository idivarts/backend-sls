package tools

import (
	"context"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

func campaignPerformance() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_campaign_performance",
			"Get aggregate performance metrics for the brand's posted campaigns over a date range.",
			openrouter.ObjectSchema(map[string]any{
				"startDate": openrouter.NumberProp("Epoch ms — start of range"),
				"endDate":   openrouter.NumberProp("Epoch ms — end of range"),
			}, []string{"startDate", "endDate"}),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			// Aggregate metrics across the brand's contents collection that are posted
			contents, err := trendlymodels.ListContentByStatus(ctx, brandID, "posted")
			if err != nil {
				return nil, err
			}

			var totalViews, totalLikes, totalComments, totalShares, count int64
			for _, ct := range contents {
				if ct.Metrics == nil {
					continue
				}
				totalViews += toInt64(ct.Metrics["views"])
				totalLikes += toInt64(ct.Metrics["likes"])
				totalComments += toInt64(ct.Metrics["comments"])
				totalShares += toInt64(ct.Metrics["shares"])
				count++
			}
			return map[string]any{
				"postCount":     count,
				"totalViews":    totalViews,
				"totalLikes":    totalLikes,
				"totalComments": totalComments,
				"totalShares":   totalShares,
			}, nil
		},
	}
}

func toInt64(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case float64:
		return int64(x)
	}
	return 0
}
