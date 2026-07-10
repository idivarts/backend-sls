package tools

import (
	"context"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

func entitlementsUsage() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_entitlements_and_usage",
			"Fetch the brand's plan, AI token-wallet balance and feature entitlements (analytics tier, inbox-reply, monthly post cap, Canva bridge). Check this before offering gated/metered actions so you can warn or suggest an upgrade instead of failing.",
			openrouter.ObjectSchema(map[string]any{}, nil),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			orgID, ok := trendlymodels.OrgIDForBrand(brandID)
			if !ok || orgID == "" {
				return map[string]any{"error": "brand is not attached to an organization"}, nil
			}
			org := &trendlymodels.Organization{}
			if err := org.Get(orgID); err != nil {
				return nil, err
			}
			planKey := ""
			if org.Billing != nil && org.Billing.PlanKey != nil {
				planKey = *org.Billing.PlanKey
			}
			ent := org.Entitlements
			if ent == nil {
				ent = trendlymodels.EntitlementsFor(planKey)
			}
			res := map[string]any{"plan": planKey}
			if ent != nil {
				res["entitlements"] = map[string]any{
					"analyticsTier":    ent.AnalyticsTier,
					"inboxReply":       ent.InboxReply,
					"maxPostsPerMonth": ent.MaxPostsPerMonth,
					"canvaBridge":      ent.CanvaBridge,
					"maxBrands":        ent.MaxBrands,
					"maxSeats":         ent.MaxSeats,
				}
			}
			if w, err := trendlymodels.GetTokenWallet(orgID); err == nil && w != nil {
				res["tokenWallet"] = map[string]any{
					"balance":          w.Balance,
					"monthlyAllotment": w.MonthlyAllotment,
					"topupBalance":     w.TopupBalance,
					"periodResetAt":    w.PeriodResetAt,
				}
			}
			return res, nil
		},
	}
}
