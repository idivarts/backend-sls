package tools

import (
	"context"

	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
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
			doc, err := firestoredb.Client.
				Collection("brands").Doc(brandID).
				Collection("strategies").Doc(id).
				Get(ctx)
			if err != nil {
				return nil, err
			}
			d := doc.Data()
			return map[string]any{
				"id":              id,
				"name":            d["name"],
				"objective":       d["objective"],
				"markdownContent": d["markdownContent"],
				"status":          d["status"],
			}, nil
		},
	}
}
