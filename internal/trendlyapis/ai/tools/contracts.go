package tools

import (
	"context"

	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

func contractHistory() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_contract_history",
			"List the brand's recent contracts.",
			openrouter.ObjectSchema(map[string]any{
				"limit": openrouter.NumberProp("Max contracts to return (default 10)"),
			}, nil),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			limit := 10
			if v, ok := args["limit"].(float64); ok && int(v) > 0 {
				limit = int(v)
			}
			iter := firestoredb.Client.Collection("contracts").
				Where("brandId", "==", brandID).
				Limit(limit).
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
					"id":     doc.Ref.ID,
					"status": d["status"],
					"userId": d["userId"],
				})
			}
			return out, nil
		},
	}
}
