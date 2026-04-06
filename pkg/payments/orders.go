package payments

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

type Order struct {
	ID         string                 `json:"id"`
	Entity     string                 `json:"entity"`
	Amount     int                    `json:"amount"`
	AmountPaid int                    `json:"amount_paid"`
	AmountDue  int                    `json:"amount_due"`
	Currency   string                 `json:"currency"`
	Receipt    string                 `json:"receipt"`
	Status     string                 `json:"status"`
	Attempts   int                    `json:"attempts"`
	Notes      map[string]interface{} `json:"notes"`
	CreatedAt  int64                  `json:"created_at"`
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

func CreateOrder(amountInRs int, notes map[string]interface{}) (*Order, error) {
	// This function will handle the creation of an order.
	// It will interact with Razorpay's API to create an order and return the order details.

	data := map[string]interface{}{
		"amount":   amountInRs * 100, // amount in paise
		"currency": "INR",
		"notes":    notes,
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
