package billing

import (
	"log"
	"time"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

// legacyPlanKeyMap maps deprecated India tier keys onto the new USD tiers, so
// migrated orgs land on a sensible new plan.
var legacyPlanKeyMap = map[string]string{
	"starter":    "pro",
	"growth":     "team",
	"enterprise": "agency",
}

// MigrateCreditWallets backfills the new credit system onto every existing org:
// it normalizes the plan key (legacy India → new USD tier), writes resolved
// entitlements + maxBrands, provisions/refills the token wallet to the plan
// allotment, and ensures an access state exists. Idempotent. When apply=false it
// only logs what it would do (dry-run). See the Credit ticket §7 Phase 8.
func MigrateCreditWallets(apply bool) error {
	orgs, err := trendlymodels.ListAllOrganizations()
	if err != nil {
		return err
	}
	nextReset := trendlymodels.NextMonthlyReset(time.Now())
	migrated, skipped := 0, 0
	for _, o := range orgs {
		if o.DeletedAt != nil {
			skipped++
			continue
		}
		planKey := resolvePlanKey(o)
		if mapped, ok := legacyPlanKeyMap[planKey]; ok {
			planKey = mapped
		}
		if _, ok := trendlymodels.PlanLimitsMap[planKey]; !ok {
			planKey = "free"
		}
		limits := trendlymodels.ResolvePlanLimits(planKey)
		log.Printf("[migrate] org=%s plan=%s allotment=%d apply=%v", o.ID, planKey, limits.MonthlyAllotment, apply)
		if !apply {
			migrated++
			continue
		}
		if err := trendlymodels.ApplyPlanToOrg(o.ID, planKey, nextReset); err != nil {
			log.Printf("[migrate] org=%s apply failed: %v", o.ID, err)
			continue
		}
		if o.Billing == nil || o.Billing.AccessState == nil {
			_ = trendlymodels.SetOrgAccessState(o.ID, "active")
		}
		migrated++
	}
	log.Printf("[migrate] done apply=%v migrated=%d skipped=%d total=%d", apply, migrated, skipped, len(orgs))
	return nil
}
