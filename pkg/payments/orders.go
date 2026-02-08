package payments

import (
	"encoding/json"
	"fmt"
	"log"
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
