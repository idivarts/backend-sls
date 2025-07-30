package payments

import "log"

func CreateOrder(amountInRs int, notes map[string]interface{}) (map[string]interface{}, error) {
	// This function will handle the creation of an order.
	// It will interact with Razorpay's API to create an order and return the order details.
	// The implementation will be added later.

	data := map[string]interface{}{
		"amount":   amountInRs * 100, // amount in paise
		"currency": "INR",
		"notes":    notes,
	}

	order, err := Client.Order.Create(data, nil)

	if err != nil {
		log.Println("Error creating order:", err)
		return nil, err
	}
	log.Println("Order created successfully:", order)

	return order, nil
}
