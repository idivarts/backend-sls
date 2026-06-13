// Package billing holds the org-level subscription/token-wallet engine that runs
// off the schedule (the 1st-of-month cron) rather than per-request. See the
// Credit ticket §5a/§7 Phase 5.
package billing

import (
	"log"
	"time"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

// RunMonthlyBilling renews token wallets and advances the billing state machine
// for every organization. Intended to run on the 1st of each month (the billing
// anchor). Decisions key off AccessState + BillingMode + PeriodEnd, so it is
// safe to run more than once.
//
// Per org:
//   - canceled / locked  → leave locked (a fresh payment reactivates it).
//   - past_due           → dunning window elapsed at the month boundary → LOCK.
//   - invoice mode       → the paid invoice month has lapsed → RE-LOCK; else leave.
//   - active / free      → RENEW: refill the wallet to the plan's monthly allotment.
func RunMonthlyBilling() error {
	orgs, err := trendlymodels.ListAllOrganizations()
	if err != nil {
		return err
	}
	now := time.Now()
	nowMs := now.UnixMilli()
	nextReset := trendlymodels.NextMonthlyReset(now)

	renewed, lockedN, skipped := 0, 0, 0
	for _, o := range orgs {
		if o.DeletedAt != nil {
			skipped++
			continue
		}
		planKey := resolvePlanKey(o)
		state, mode, periodEnd := billingState(o)

		switch {
		case state == "canceled" || state == "locked":
			skipped++

		case state == "past_due":
			if err := trendlymodels.SetOrgAccessState(o.ID, "locked"); err != nil {
				log.Println("billing cron: lock past_due failed", o.ID, err)
			} else {
				lockedN++
			}

		case mode == "invoice":
			if periodEnd > 0 && nowMs >= periodEnd {
				if err := trendlymodels.SetOrgAccessState(o.ID, "locked"); err != nil {
					log.Println("billing cron: relock invoice failed", o.ID, err)
				} else {
					lockedN++
				}
			} else {
				skipped++
			}

		default: // active recurring OR free → renew the monthly allotment
			limits := trendlymodels.ResolvePlanLimits(planKey)
			if err := trendlymodels.RefillWallet(o.ID, limits.MonthlyAllotment, nextReset); err != nil {
				log.Println("billing cron: refill failed", o.ID, err)
				continue
			}
			renewed++
		}
	}
	log.Printf("billing cron: orgs=%d renewed=%d locked=%d skipped=%d", len(orgs), renewed, lockedN, skipped)
	return nil
}

func resolvePlanKey(o trendlymodels.OrganizationWithID) string {
	if o.Billing != nil && o.Billing.PlanKey != nil && *o.Billing.PlanKey != "" {
		return *o.Billing.PlanKey
	}
	if o.PlanKey != nil && *o.PlanKey != "" {
		return *o.PlanKey
	}
	return "free"
}

func billingState(o trendlymodels.OrganizationWithID) (state, mode string, periodEnd int64) {
	state, mode = "active", "recurring"
	if o.Billing != nil {
		if o.Billing.AccessState != nil && *o.Billing.AccessState != "" {
			state = *o.Billing.AccessState
		}
		if o.Billing.BillingMode != nil && *o.Billing.BillingMode != "" {
			mode = *o.Billing.BillingMode
		}
		if o.Billing.PeriodEnd != nil {
			periodEnd = *o.Billing.PeriodEnd
		}
	}
	return
}
