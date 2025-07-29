package payments

import (
	"os"

	razorpay "github.com/razorpay/razorpay-go"
)

var (
	Client    *razorpay.Client
	apiKey    = ""
	apiSecret = ""
)

func init() {
	apiKey = os.Getenv("RAZORPAY_API_KEY")
	apiSecret = os.Getenv("RAZORPAY_API_SECRET")

	if apiKey == "" || apiSecret == "" {
		apiKey = "rzp_test_Z9T0fM1E1agkpR"
		apiSecret = "LaqAVYPBdqdrC4psaoga18nE"
	}

	Client = razorpay.NewClient(apiKey, apiSecret)
}
