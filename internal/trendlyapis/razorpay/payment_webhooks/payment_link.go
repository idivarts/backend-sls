package paymentwebhooks

import (
	"errors"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/myutil"
	"github.com/idivarts/backend-sls/pkg/payments/webhook"
)

func handlePaymentLink(event *webhook.Event) error {
	if event.Payload.PaymentLink == nil {
		return errors.New("payment_link payload missing")
	}

	paymentLink := event.Payload.PaymentLink.Entity
	if paymentLink.Notes.BrandID == "" {
		return errors.New("brandid-null")
	}

	brand := &trendlymodels.Brand{}
	err := brand.Get(paymentLink.Notes.BrandID)
	if err != nil {
		return err
	}
	if brand.Billing == nil {
		brand.Billing = &trendlymodels.BrandBilling{}
	}

	if brand.Billing.PaymentLinkId != nil && brand.Billing.PaymentLinkId != &paymentLink.ID &&
		paymentLink.Status != "paid" &&
		brand.Billing.Status != nil && *brand.Billing.Status == 1 {
		return errors.New("payment-link-cant-be-replaced-unless-active")
	}

	brand.Billing.PaymentLinkId = &paymentLink.ID
	brand.Billing.BillingStatus = myutil.StrPtr("active")
	if paymentLink.Notes.PlanKey != "" {
		brand.Billing.PlanKey = &paymentLink.Notes.PlanKey
	}
	if paymentLink.Notes.PlanCycle != "" {
		brand.Billing.PlanCycle = &paymentLink.Notes.PlanCycle
	}

	switch paymentLink.Status {
	case "paid":
		brand.Billing.IsOnTrial = myutil.BoolPtr(false)
		brand.Billing.Status = myutil.IntPtr(1)
	default:
		brand.Billing.IsOnTrial = myutil.BoolPtr(true)
		brand.Billing.Status = myutil.IntPtr(0)
	}

	if event.Event == "payment_link.paid" {
		bCredit, b := trendlymodels.PlanCreditsMap[*brand.Billing.PlanKey]
		if b {
			mult := 1
			if *brand.Billing.PlanKey == "yearly" {
				mult = 12
			}
			brand.Credits.Discovery += (bCredit.Discovery * mult)
			brand.Credits.Collaboration += (bCredit.Collaboration * mult)
			brand.Credits.Connection += (bCredit.Connection * mult)
			brand.Credits.Contract += (bCredit.Contract * mult)
			brand.Credits.Influencer += (bCredit.Influencer * mult)
		}
	}

	_, err = brand.Insert(paymentLink.Notes.BrandID)
	if err != nil {
		return err
	}

	return nil
}
