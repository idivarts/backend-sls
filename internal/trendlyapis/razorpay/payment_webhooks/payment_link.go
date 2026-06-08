package paymentwebhooks

import (
	"errors"

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

	// Old per-brand credit top-up on payment_link.paid removed — the new org
	// token wallet is handled by the Credit ticket's billing engine, not here.

	return target.save(billing)
}
