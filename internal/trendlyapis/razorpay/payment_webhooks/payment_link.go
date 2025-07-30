package paymentwebhooks

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
	Description           string                  `json:"description"`
	ExpireBy              int64                   `json:"expire_by"`
	ExpiredAt             int64                   `json:"expired_at"`
	FirstMinPartialAmount int64                   `json:"first_min_partial_amount"`
	ID                    string                  `json:"id"`
	Notes                 *map[string]interface{} `json:"notes"` // nullable
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

func HandlePaymentLink(event RazorpayWebhookEvent) {

}
