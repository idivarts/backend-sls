package tools

import (
	"context"

	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
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
			iter := firestoredb.Client.
				Collection("brands").Doc(brandID).
				Collection("contents").
				Where("status", "==", "posted").
				Documents(ctx)
			defer iter.Stop()

			var totalViews, totalLikes, totalComments, totalShares, count int64
			for {
				doc, err := iter.Next()
				if err != nil {
					break
				}
				data := doc.Data()
				metrics, ok := data["metrics"].(map[string]any)
				if !ok {
					continue
				}
				totalViews += toInt64(metrics["views"])
				totalLikes += toInt64(metrics["likes"])
				totalComments += toInt64(metrics["comments"])
				totalShares += toInt64(metrics["shares"])
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
