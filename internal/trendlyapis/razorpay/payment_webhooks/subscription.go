package paymentwebhooks

import (
	"errors"
	"log"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/myutil"
	"github.com/idivarts/backend-sls/pkg/payments"
	"github.com/idivarts/backend-sls/pkg/payments/webhook"
)

func HandleSubscription(event *webhook.Event) error {
	if event.Payload.Subscription == nil {
		return errors.New("subscription payload missing")
	}

	subscription := event.Payload.Subscription.Entity

	if subscription.Notes.BrandID == "" {
		return errors.New("brandid-null")
	}

	brand := &trendlymodels.Brand{}
	err := brand.Get(subscription.Notes.BrandID)
	if err != nil {
		return err
	}
	if brand.Billing == nil {
		brand.Billing = &trendlymodels.BrandBilling{}
	}

	if brand.Billing.Subscription != nil && brand.Billing.Subscription != &subscription.ID &&
		subscription.Status != "active" &&
		brand.Billing.Status != nil && *brand.Billing.Status == 1 {
		return errors.New("subscription-cant-be-replaced-unless-active")
	}

	if brand.Billing.Subscription != nil && *brand.Billing.Subscription != "" && *brand.Billing.Subscription != subscription.ID {
		subscriptionID := *brand.Billing.Subscription
		defer func() {
			_, err := payments.CancelSubscription(subscriptionID, false)
			if err != nil {
				log.Println("Unable to cancel previous subscription", subscriptionID, err)
			}
		}()
	}

	brand.Billing.Subscription = &subscription.ID
	brand.Billing.SubscriptionUrl = subscription.ShortURL
	brand.Billing.BillingStatus = &subscription.Status
	if subscription.Notes.PlanKey != "" {
		brand.Billing.PlanKey = &subscription.Notes.PlanKey
	}
	if subscription.Notes.PlanCycle != "" {
		brand.Billing.PlanCycle = &subscription.Notes.PlanCycle
	}

	switch *brand.Billing.BillingStatus {
	case "created":
		brand.Billing.Status = myutil.IntPtr(0)
	case "authenticated":
		brand.Billing.IsOnTrial = myutil.BoolPtr(true)
		brand.Billing.Status = myutil.IntPtr(1)
	case "active":
		brand.Billing.IsOnTrial = myutil.BoolPtr(false)
		brand.Billing.Status = myutil.IntPtr(1)
	case "pending":
	case "completed":
		brand.Billing.Status = myutil.IntPtr(5)
	case "halted":
		brand.Billing.Status = myutil.IntPtr(2)
	case "cancelled":
		brand.Billing.Status = myutil.IntPtr(3)
	}

	log.Println("Updating Brand Subscription Status to", event.Event, *brand.Billing.BillingStatus, brand.Billing.PlanKey)
	if event.Event == "subscription.charged" || event.Event == "subscription.authenticated" {
		bCredit, b := trendlymodels.PlanCreditsMap[*brand.Billing.PlanKey]
		if b {
			mult := 1
			if *brand.Billing.PlanKey == "yearly" {
				mult = 12
			}
			brand.Credits.Discovery = (bCredit.Discovery * mult)
			brand.Credits.Collaboration = (bCredit.Collaboration * mult)
			brand.Credits.Connection = (bCredit.Connection * mult)
			brand.Credits.Contract = (bCredit.Contract * mult)
			brand.Credits.Influencer = (bCredit.Influencer * mult)
		}
	}

	_, err = brand.Insert(subscription.Notes.BrandID)
	if err != nil {
		return err
	}

	return nil
}
