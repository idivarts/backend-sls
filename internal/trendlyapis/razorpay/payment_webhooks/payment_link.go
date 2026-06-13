package paymentwebhooks

import (
	"errors"
	"log"
	"time"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/myutil"
	"github.com/idivarts/backend-sls/pkg/payments/webhook"
)

func handlePaymentLink(event *webhook.Event) error {
	if event.Payload.PaymentLink == nil {
		return errors.New("payment_link payload missing")
	}

	paymentLink := event.Payload.PaymentLink.Entity
	if paymentLink.Notes.BrandID == "" && paymentLink.Notes.OrganizationID == "" {
		return errors.New("billing-target-null")
	}

	// Billing lives on the Organization now; resolve the org (preferred) + brand
	// (mirror) the webhook applies to.
	target, err := resolveBillingTarget(paymentLink.Notes)
	if err != nil {
		return err
	}
	billing := target.currentBilling()

	if billing.PaymentLinkId != nil && billing.PaymentLinkId != &paymentLink.ID &&
		paymentLink.Status != "paid" &&
		billing.Status != nil && *billing.Status == 1 {
		return errors.New("payment-link-cant-be-replaced-unless-active")
	}

	billing.PaymentLinkId = &paymentLink.ID
	billing.BillingStatus = myutil.StrPtr("active")
	if paymentLink.Notes.PlanKey != "" {
		billing.PlanKey = &paymentLink.Notes.PlanKey
	}
	if paymentLink.Notes.PlanCycle != "" {
		billing.PlanCycle = &paymentLink.Notes.PlanCycle
	}

	switch paymentLink.Status {
	case "paid":
		billing.IsOnTrial = myutil.BoolPtr(false)
		billing.Status = myutil.IntPtr(1)
	default:
		billing.IsOnTrial = myutil.BoolPtr(true)
		billing.Status = myutil.IntPtr(0)
	}

	// The invoice / manual-pay fallback: a PAID payment-link grants exactly one
	// month of access (billingMode=invoice) and funds the wallet. The cron
	// re-locks the org when this paid month lapses unless another invoice is paid.
	if paymentLink.Status == "paid" {
		reset := trendlymodels.NextMonthlyReset(time.Now())
		billing.AccessState = myutil.StrPtr("active")
		billing.BillingMode = myutil.StrPtr("invoice")
		billing.BillingAnchorDay = myutil.IntPtr(1)
		billing.Provider = myutil.StrPtr("razorpay")
		billing.PeriodEnd = &reset
		planKey := paymentLink.Notes.PlanKey
		if planKey == "" && billing.PlanKey != nil {
			planKey = *billing.PlanKey
		}
		if planKey != "" {
			if err := trendlymodels.ApplyPlanToOrg(target.orgID, planKey, reset); err != nil {
				log.Println("apply plan / wallet refill (invoice) failed", target.orgID, err)
			}
		}
	}

	return target.save(billing)
}
