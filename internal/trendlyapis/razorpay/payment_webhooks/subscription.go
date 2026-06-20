package paymentwebhooks

import (
	"errors"
	"log"
	"time"

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

	// Drive OUR app-level access state + fund the org token wallet. Each "active"
	// webhook (subscription.activated + each monthly subscription.charged) refills
	// the wallet to the plan's monthly allotment and refreshes entitlements.
	switch subscription.Status {
	case "active":
		reset := trendlymodels.NextMonthlyReset(time.Now())
		billing.AccessState = myutil.StrPtr("active")
		billing.BillingMode = myutil.StrPtr("recurring")
		billing.BillingAnchorDay = myutil.IntPtr(1)
		billing.Provider = myutil.StrPtr("razorpay")
		billing.PeriodEnd = &reset
		planKey := subscription.Notes.PlanKey
		if planKey == "" && billing.PlanKey != nil {
			planKey = *billing.PlanKey
		}
		if planKey != "" {
			if err := trendlymodels.ApplyPlanToOrg(target.orgID, planKey, reset); err != nil {
				log.Println("apply plan / wallet refill failed", target.orgID, err)
			}
		}
	case "halted":
		billing.AccessState = myutil.StrPtr("past_due")
	case "cancelled":
		billing.AccessState = myutil.StrPtr("canceled")
	}

	log.Println("Updating Subscription Status to", event.Event, *billing.BillingStatus, billing.PlanKey)

	return target.save(billing)
}
