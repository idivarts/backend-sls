package paymentwebhooks

import (
	"errors"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/myutil"
)

type PaymentLinkEntity struct {
	AcceptPartial bool   `json:"accept_partial"`
	Amount        int64  `json:"amount"`
	AmountPaid    int64  `json:"amount_paid"`
	CancelledAt   int64  `json:"cancelled_at"`
	CreatedAt     int64  `json:"created_at"`
	Currency      string `json:"currency"`
	Customer      struct {
		Contact string `json:"contact"`
		Email   string `json:"email"`
	} `json:"customer"`
	Description           string            `json:"description"`
	ExpireBy              int64             `json:"expire_by"`
	ExpiredAt             int64             `json:"expired_at"`
	FirstMinPartialAmount int64             `json:"first_min_partial_amount"`
	ID                    string            `json:"id"`
	Notes                 SubscriptionNotes `json:"notes"` // nullable
	Notify                struct {
		Email    bool `json:"email"`
		SMS      bool `json:"sms"`
		WhatsApp bool `json:"whatsapp"`
	} `json:"notify"`
	OrderID        string `json:"order_id"`
	ReferenceID    string `json:"reference_id"`
	ReminderEnable bool   `json:"reminder_enable"`
	Reminders      struct {
		Status string `json:"status"`
	} `json:"reminders"`
	ShortURL     string `json:"short_url"`
	Status       string `json:"status"`
	UpdatedAt    int64  `json:"updated_at"`
	UPILink      bool   `json:"upi_link"`
	UserID       string `json:"user_id"`
	WhatsAppLink bool   `json:"whatsapp_link"`
}

// Accepted Value for Status
// created
// partially_paid
// expired
// cancelled
// paid

func handlePaymentLink(event RazorpayWebhookEvent) error {
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
	// brand.Billing.SubscriptionUrl = subscription.ShortURL
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
		break
	default:
		brand.Billing.IsOnTrial = myutil.BoolPtr(true)
		brand.Billing.Status = myutil.IntPtr(0)
		break
	}

	_, err = brand.Insert(paymentLink.Notes.BrandID)
	if err != nil {
		return err
	}

	return nil
}
