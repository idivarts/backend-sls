package tools

import (
	"context"

	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

func influencerStats() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_influencer_stats",
			"Fetch profile stats for a specific influencer by user ID.",
			openrouter.ObjectSchema(map[string]any{
				"influencerId": openrouter.StringProp("The influencer user ID"),
			}, []string{"influencerId"}),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			id, _ := args["influencerId"].(string)
			if id == "" {
				return nil, nil
			}
			doc, err := firestoredb.Client.Collection("users").Doc(id).Get(ctx)
			if err != nil {
				return nil, err
			}
			d := doc.Data()
			return map[string]any{
				"id":       id,
				"name":     d["name"],
				"location": d["location"],
				"profile":  d["profile"],
			}, nil
		},
	}
}

func searchInfluencers() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"search_influencers",
			"Search the brand's discovered influencers by niche.",
			openrouter.ObjectSchema(map[string]any{
				"niche": openrouter.StringProp("Niche keyword (e.g. fashion, fitness, food)"),
				"limit": openrouter.NumberProp("Maximum results to return (default 20)"),
			}, []string{"niche"}),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			// Placeholder: brand-scoped influencer search would query BigQuery/RDS.
			// For now return the brand's discovered influencer IDs.
			doc, err := firestoredb.Client.Collection("brands").Doc(brandID).Get(ctx)
			if err != nil {
				return nil, err
			}
			ids, _ := doc.Data()["discoveredInfluencers"].([]any)
			limit := 20
			if v, ok := args["limit"].(float64); ok && int(v) > 0 {
				limit = int(v)
			}
			if len(ids) > limit {
				ids = ids[:limit]
			}
			return map[string]any{"influencerIds": ids}, nil
		},
	}
}
