package payments

import "log"

type Customer struct {
	Name        string
	Email       string
	PhoneNumber string
}

func CreatePaymentLink(amountInRs int, contact Customer, notes map[string]interface{}) (string, error) {
	linkData := map[string]interface{}{
		"amount":   amountInRs * 100,
		"currency": "INR",
		"customer": map[string]interface{}{
			"name":    contact.Name,
			"email":   contact.Email,
			"contact": contact.PhoneNumber,
		},
		"notes":           notes,
		"callback_url":    RedirectUrl,
		"callback_method": "get",
	}
	link, err := Client.PaymentLink.Create(linkData, nil)
	if err != nil {
		return "", err
	}
	log.Println("Link", link)
	return link["short_url"].(string), err
}
