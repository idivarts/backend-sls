package payments

import (
	razorpay "github.com/razorpay/razorpay-go"
)

var (
	Client     *razorpay.Client
	apiKey     = ""
	apiSecret  = ""
	webhookKey = ""
)

func init() {
	// Write code to get the items from json file key-secrets.json

	if apiKey == "" || apiSecret == "" {
		apiKey = "rzp_test_Z9T0fM1E1agkpR"
		apiSecret = "LaqAVYPBdqdrC4psaoga18nE"
		webhookKey = "rzp_test_webhook_1234567890"
	}

	Client = razorpay.NewClient(apiKey, apiSecret)
}
