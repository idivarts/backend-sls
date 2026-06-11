package tools

import (
	"context"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

func strategyContent() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_strategy_content",
			"Fetch the markdown body of a strategy document by ID.",
			openrouter.ObjectSchema(map[string]any{
				"strategyId": openrouter.StringProp("Strategy document ID"),
			}, []string{"strategyId"}),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			id, _ := args["strategyId"].(string)
			if id == "" {
				return nil, nil
			}
			strat, err := trendlymodels.GetStrategy(ctx, brandID, id)
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"id":              strat.ID,
				"name":            strat.Name,
				"objective":       strat.Objective,
				"markdownContent": strat.MarkdownContent,
				"status":          strat.Status,
			}, nil
		},
	}
}
