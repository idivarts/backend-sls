package payments

import (
	"log"
	"os"
	"strings"
)

type Customer struct {
	Name        string
	Email       string
	PhoneNumber string
}

// DefaultBillingCurrency is the currency for ORG SUBSCRIPTION billing (USD once
// Razorpay International is enabled; override via BILLING_CURRENCY). Domestic
// brand→influencer contract payments stay INR and keep using CreatePaymentLink /
// CreateOrder unchanged — only the subscription/invoice path uses this.
func DefaultBillingCurrency() string {
	if c := strings.TrimSpace(os.Getenv("BILLING_CURRENCY")); c != "" {
		return strings.ToUpper(c)
	}
	return "USD"
}

// CreatePaymentLinkCurrency creates a payment link in an explicit currency — used
// by the subscription invoice / manual-pay fallback (USD). amountMajor is in the
// currency's major unit (dollars/rupees); Razorpay is sent the minor unit.
func CreatePaymentLinkCurrency(amountMajor int, currency string, contact Customer, notes map[string]interface{}) (string, string, error) {
	linkData := map[string]interface{}{
		"amount":   amountMajor * 100,
		"currency": currency,
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
		return "", "", err
	}
	return link["id"].(string), link["short_url"].(string), err
}

func CreatePaymentLink(amountInRs int, contact Customer, notes map[string]interface{}) (string, string, error) {
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
		return "", "", err
	}
	log.Println("Link", link)
	return link["id"].(string), link["short_url"].(string), err
}
