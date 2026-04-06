package payments

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

type Order struct {
	ID         string                  `json:"id"`
	Entity     string                  `json:"entity"`
	Amount     int                     `json:"amount"`
	AmountPaid int                     `json:"amount_paid"`
	AmountDue  int                     `json:"amount_due"`
	Currency   string                  `json:"currency"`
	Receipt    string                  `json:"receipt"`
	Status     string                  `json:"status"`
	Attempts   int                     `json:"attempts"`
	Notes      map[string]interface{}  `json:"notes"`
	CreatedAt  int64                   `json:"created_at"`
	Transfers  []OrderTransferResponse `json:"transfers,omitempty"`
}

// OrderTransferError is the nested error object on transfers returned with an order.
type OrderTransferError struct {
	Code        *string     `json:"code"`
	Description *string     `json:"description"`
	Reason      *string     `json:"reason"`
	Field       *string     `json:"field"`
	Step        *string     `json:"step"`
	ID          *string     `json:"id"`
	Source      *string     `json:"source"`
	Metadata    interface{} `json:"metadata"`
}

// OrderTransferResponse is one Route transfer returned on order create/fetch (recipient, status, etc.).
type OrderTransferResponse struct {
	ID                    string                 `json:"id"`
	Entity                string                 `json:"entity"`
	Status                string                 `json:"status"`
	Source                string                 `json:"source"`
	Recipient             string                 `json:"recipient"`
	Amount                int                    `json:"amount"`
	Currency              string                 `json:"currency"`
	AmountReversed        int                    `json:"amount_reversed"`
	Notes                 map[string]interface{} `json:"notes,omitempty"`
	LinkedAccountNotes    []string               `json:"linked_account_notes,omitempty"`
	OnHold                bool                   `json:"on_hold"`
	OnHoldUntil           *int64                 `json:"on_hold_until,omitempty"`
	RecipientSettlementID *string                `json:"recipient_settlement_id,omitempty"`
	CreatedAt             int64                  `json:"created_at"`
	ProcessedAt           *int64                 `json:"processed_at,omitempty"`
	Error                 *OrderTransferError    `json:"error,omitempty"`
}

func MapToOrder(m map[string]interface{}) (*Order, error) {
	bytes, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal map to order: %w", err)
	}

	var order Order
	if err := json.Unmarshal(bytes, &order); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON to Order struct: %w", err)
	}

	return &order, nil
}

// OrderTransfer is one linked-account transfer embedded in order creation (Razorpay Route / order API).
// Amount is in the smallest currency unit (paise for INR), consistent with the order amount field.
type OrderTransfer struct {
	Account            string                 `json:"account"`
	Amount             int                    `json:"amount"`
	Currency           string                 `json:"currency"`
	Notes              map[string]interface{} `json:"notes,omitempty"`
	LinkedAccountNotes []string               `json:"linked_account_notes,omitempty"`
	OnHold             *bool                  `json:"on_hold,omitempty"`
	OnHoldUntil        *int64                 `json:"on_hold_until,omitempty"`
}

func CreateOrder(amountInRs int, notes map[string]interface{}, transfers []OrderTransfer) (*Order, error) {
	// This function will handle the creation of an order.
	// It will interact with Razorpay's API to create an order and return the order details.
	// Pass nil or an empty transfers slice to omit "transfers" from the payload.

	data := map[string]interface{}{
		"amount":   amountInRs * 100, // amount in paise
		"currency": "INR",
		"notes":    notes,
	}

	if len(transfers) > 0 {
		payload := make([]map[string]interface{}, 0, len(transfers))
		for i := range transfers {
			t := transfers[i]
			m := map[string]interface{}{
				"account":  t.Account,
				"amount":   t.Amount * 100,
				"currency": "INR",
			}
			if len(t.Notes) > 0 {
				m["notes"] = t.Notes
			}
			if len(t.LinkedAccountNotes) > 0 {
				m["linked_account_notes"] = t.LinkedAccountNotes
			}
			if t.OnHold != nil {
				m["on_hold"] = *t.OnHold
			}
			if t.OnHoldUntil != nil {
				m["on_hold_until"] = *t.OnHoldUntil
			}
			payload = append(payload, m)
		}
		data["transfers"] = payload
	}

	res, err := Client.Order.Create(data, nil)

	if err != nil {
		log.Println("Error creating order:", err)
		return nil, err
	}
	log.Println("Order created successfully:", res)

	return MapToOrder(res)
}

func FetchOrder(orderID string) (*Order, error) {
	order, err := Client.Order.Fetch(orderID, nil, nil)
	if err != nil {
		log.Println("Error fetching order:", err)
		return nil, err
	}
	return MapToOrder(order)
}

// OrderIDFromTransferSource resolves the Razorpay order id referenced by a transfer's `source` field.
// Route transfers may use source order_* or, when tied to a capture, payment id pay_* (order is read from the payment).
func OrderIDFromTransferSource(source string) (string, error) {
	s := strings.TrimSpace(source)
	switch {
	case s == "":
		return "", fmt.Errorf("empty transfer source")
	case strings.HasPrefix(s, "order_"):
		return s, nil
	case strings.HasPrefix(s, "pay_"):
		res, err := Client.Payment.Fetch(s, nil, nil)
		if err != nil {
			return "", fmt.Errorf("fetch payment %s: %w", s, err)
		}
		raw, ok := res["order_id"]
		if !ok || raw == nil {
			return "", fmt.Errorf("payment %s has no order_id", s)
		}
		oid, ok := raw.(string)
		if !ok || oid == "" {
			return "", fmt.Errorf("payment %s order_id is not a non-empty string", s)
		}
		return oid, nil
	default:
		return "", fmt.Errorf("unsupported transfer source %q (expected order_* or pay_*)", s)
	}
}
