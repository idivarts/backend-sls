package tools

import (
	"context"

	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

func activeCollaborations() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_active_collaborations",
			"List the brand's active collaborations (status filter optional).",
			openrouter.ObjectSchema(map[string]any{
				"status": openrouter.EnumProp("Optional collaboration status filter", []string{"draft", "active", "closed"}),
			}, nil),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			q := firestoredb.Client.Collection("collaborations").Where("brandId", "==", brandID)
			if status, ok := args["status"].(string); ok && status != "" {
				q = q.Where("status", "==", status)
			}
			iter := q.Limit(20).Documents(ctx)
			defer iter.Stop()

			var out []map[string]any
			for {
				doc, err := iter.Next()
				if err != nil {
					break
				}
				data := doc.Data()
				out = append(out, map[string]any{
					"id":     doc.Ref.ID,
					"name":   data["name"],
					"status": data["status"],
				})
			}
			return out, nil
		},
	}
}
