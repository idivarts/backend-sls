package paymentwebhooks

import (
	"errors"
	"log"

	"github.com/idivarts/backend-sls/pkg/myutil"
	"github.com/idivarts/backend-sls/pkg/payments"
	"github.com/idivarts/backend-sls/pkg/payments/webhook"
)

func HandleSubscription(event *webhook.Event) error {
	if event.Payload.Subscription == nil {
		return errors.New("subscription payload missing")
	}

	subscription := event.Payload.Subscription.Entity

	if subscription.Notes.BrandID == "" && subscription.Notes.OrganizationID == "" {
		return errors.New("billing-target-null")
	}

	// Billing lives on the Organization now; resolve the org (preferred) + brand
	// (mirror) the webhook applies to.
	target, err := resolveBillingTarget(subscription.Notes)
	if err != nil {
		return err
	}
	billing := target.currentBilling()

	if billing.Subscription != nil && billing.Subscription != &subscription.ID &&
		subscription.Status != "active" &&
		billing.Status != nil && *billing.Status == 1 {
		return errors.New("subscription-cant-be-replaced-unless-active")
	}

	if billing.Subscription != nil && *billing.Subscription != "" && *billing.Subscription != subscription.ID {
		subscriptionID := *billing.Subscription
		defer func() {
			_, err := payments.CancelSubscription(subscriptionID, false)
			if err != nil {
				log.Println("Unable to cancel previous subscription", subscriptionID, err)
			}
		}()
	}

	billing.Subscription = &subscription.ID
	billing.SubscriptionUrl = subscription.ShortURL
	billing.BillingStatus = &subscription.Status
	if subscription.Notes.PlanKey != "" {
		billing.PlanKey = &subscription.Notes.PlanKey
	}
	if subscription.Notes.PlanCycle != "" {
		billing.PlanCycle = &subscription.Notes.PlanCycle
	}

	switch *billing.BillingStatus {
	case "created":
		billing.Status = myutil.IntPtr(0)
	case "authenticated":
		billing.IsOnTrial = myutil.BoolPtr(true)
		billing.Status = myutil.IntPtr(1)
	case "active":
		billing.IsOnTrial = myutil.BoolPtr(false)
		billing.Status = myutil.IntPtr(1)
	case "pending":
	case "completed":
		billing.Status = myutil.IntPtr(5)
	case "halted":
		billing.Status = myutil.IntPtr(2)
	case "cancelled":
		billing.Status = myutil.IntPtr(3)
	}

	log.Println("Updating Subscription Status to", event.Event, *billing.BillingStatus, billing.PlanKey)
	// Old per-brand credit allotment on charge/auth removed — the new org token
	// wallet is refilled by the Credit ticket's billing engine, not here.

	return target.save(billing)
}
