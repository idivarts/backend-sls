package paymentwebhooks

import (
	"errors"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/myutil"
)

type SubscriptionNotes struct {
	BrandID      string `json:"brandId"`
	PlanName     string `json:"planName"`
	IsGrowthPlan bool   `json:"isGrowthPlan"`
}
type SubscriptionEntity struct {
	ID                  string            `json:"id"`
	Entity              string            `json:"entity"`
	PlanID              string            `json:"plan_id"`
	CustomerID          string            `json:"customer_id"`
	Status              string            `json:"status"`
	CurrentStart        int64             `json:"current_start"`
	CurrentEnd          int64             `json:"current_end"`
	EndedAt             *int64            `json:"ended_at"` // nullable
	Quantity            int               `json:"quantity"`
	Notes               SubscriptionNotes `json:"notes"`
	ChargeAt            int64             `json:"charge_at"`
	StartAt             int64             `json:"start_at"`
	EndAt               int64             `json:"end_at"`
	AuthAttempts        int               `json:"auth_attempts"`
	TotalCount          int               `json:"total_count"`
	PaidCount           int               `json:"paid_count"`
	CustomerNotify      bool              `json:"customer_notify"`
	CreatedAt           int64             `json:"created_at"`
	ExpireBy            int64             `json:"expire_by"`
	ShortURL            *string           `json:"short_url"` // nullable
	HasScheduledChanges bool              `json:"has_scheduled_changes"`
	ChangeScheduledAt   *int64            `json:"change_scheduled_at"` // nullable
	Source              string            `json:"source"`
	OfferID             string            `json:"offer_id"`
	RemainingCount      int               `json:"remaining_count"`
}

func handleSubscription(event RazorpayWebhookEvent) error {

	subscription := event.Payload.Subscription.Entity

	if subscription.Notes.BrandID == "" {
		return errors.New("brandid-null")
	}

	brand := &trendlymodels.Brand{}
	err := brand.Get(subscription.Notes.BrandID)
	if err != nil {
		return err
	}

	brand.Billing.Subscription = &subscription.ID
	brand.Billing.BillingStatus = &subscription.Status
	switch *brand.Billing.BillingStatus {
	case "created":
		brand.Billing.Status = myutil.IntPtr(0)
		break
	case "authenticated":
	case "active":
		brand.Billing.Status = myutil.IntPtr(1)
		break
	case "pending":
	case "completed":
		brand.Billing.Status = myutil.IntPtr(5)
		break
	case "halted":
		brand.Billing.Status = myutil.IntPtr(2)
		break
	case "cancelled":
		brand.Billing.Status = myutil.IntPtr(3)
		break
	}

	_, err = brand.Insert(subscription.Notes.BrandID)
	if err != nil {
		return err
	}

	return nil
}
