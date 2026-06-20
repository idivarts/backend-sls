package webhook

// PaymentEntity is a subset of Razorpay's payment resource. Only fields used by
// this codebase are unmarshaled; other JSON keys are ignored to avoid brittle
// parsing when Razorpay changes shapes (e.g. string vs object).
type PaymentEntity struct {
	ID          string  `json:"id"`
	OrderID     string  `json:"order_id"`
	ErrorReason *string `json:"error_reason"`

	/*
		Unused for now — uncomment if you need them (may require type fixes vs live payloads):

		AcquirerData struct {
			RRN string `json:"rrn"`
		} `json:"acquirer_data"`
		Amount            int64                  `json:"amount"`
		AmountRefunded    int64                  `json:"amount_refunded"`
		AmountTransferred int64                  `json:"amount_transferred"`
		Bank              *string                `json:"bank"`
		BaseAmount        int64                  `json:"base_amount"`
		Captured          bool                   `json:"captured"`
		Card              json.RawMessage        `json:"card"`
		CardID            *string                `json:"card_id"`
		Contact           string                 `json:"contact"`
		CreatedAt         int64                  `json:"created_at"`
		Currency          string                 `json:"currency"`
		Description       string                 `json:"description"`
		Email             *string                `json:"email"`
		Entity            string                 `json:"entity"`
		ErrorCode         *string                `json:"error_code"`
		ErrorDescription *string `json:"error_description"`
		ErrorSource       *string                `json:"error_source"`
		ErrorStep         *string                `json:"error_step"`
		Fee               int64                  `json:"fee"`
		FeeBearer         string                 `json:"fee_bearer"`
		International     bool                   `json:"international"`
		InvoiceID         *string                `json:"invoice_id"`
		Method            string                 `json:"method"`
		Notes             map[string]interface{} `json:"notes"`
		RefundStatus      *string                `json:"refund_status"`
		Status            string                 `json:"status"`
		Tax               int64                  `json:"tax"`
		UPI               *struct {
			PayerAccountType string `json:"payer_account_type"`
			VPA              string `json:"vpa"`
		} `json:"upi"`
		VPA    string  `json:"vpa"`
		Wallet *string `json:"wallet"`
	*/
}
