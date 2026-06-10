package ai

import (
	"context"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// brandPlan resolves a brand's subscription plan (free/pro/team/agency) from its
// organization's billing. Falls back to free when anything is missing.
func brandPlan(brandID string) openrouter.Plan {
	if brandID == "" {
		return openrouter.PlanFree
	}
	b, err := loadBrand(brandID)
	if err != nil || b.OrganizationID == nil || *b.OrganizationID == "" {
		return openrouter.PlanFree
	}
	org := &trendlymodels.Organization{}
	if err := org.Get(*b.OrganizationID); err != nil {
		return openrouter.PlanFree
	}
	if org.Billing != nil && org.Billing.PlanKey != nil {
		return openrouter.PlanFromKey(*org.Billing.PlanKey)
	}
	return openrouter.PlanFree
}

// pickModel resolves the model to use for a task, enforcing per-plan gating from
// the Firestore-backed registry. locked=true means the brand's plan unlocks no
// model allowed for this task (the caller must surface an upgrade prompt instead
// of running the model).
func pickModel(ctx context.Context, brandID string, task openrouter.TaskType, requested string) (model string, locked bool) {
	openrouter.EnsureRegistry(ctx)
	return openrouter.Resolve(task, brandPlan(brandID), requested)
}
