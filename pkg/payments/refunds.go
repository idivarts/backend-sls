package payments

import (
	"encoding/json"
	"fmt"
	"log"
)

type Refund struct {
	ID        string `json:"id"`
	Entity    string `json:"entity"`
	Amount    int    `json:"amount"`
	Currency  string `json:"currency"`
	PaymentID string `json:"payment_id"`
	Status    string `json:"status"`
	Notes     map[string]interface{} `json:"notes,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

func mapToRefund(m map[string]interface{}) (*Refund, error) {
	bytes, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal map to refund: %w", err)
	}
	var r Refund
	if err := json.Unmarshal(bytes, &r); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON to Refund struct: %w", err)
	}
	return &r, nil
}

// CreateRefund issues a partial or full refund for a Razorpay payment.
// amountInPaise is the amount to refund in paise (100 paise = ₹1).
// Pass the full payment.Amount*100 for a full refund.
func CreateRefund(paymentID string, amountInPaise int64, reason string) (*Refund, error) {
	data := map[string]interface{}{
		"amount": amountInPaise,
		"notes": map[string]interface{}{
			"reason": reason,
		},
	}

	res, err := Client.Payment.Refund(paymentID, int(amountInPaise), data, nil)
	if err != nil {
		log.Printf("Error creating refund for payment %s: %v", paymentID, err)
		return nil, err
	}

	log.Printf("Refund created successfully for payment %s", paymentID)
	return mapToRefund(res)
}
