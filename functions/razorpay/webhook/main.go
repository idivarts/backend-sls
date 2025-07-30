package main

import (
	paymentwebhooks "github.com/idivarts/backend-sls/internal/trendlyapis/razorpay/payment_webhooks"
	apihandler "github.com/idivarts/backend-sls/pkg/api_handler"
)

func main() {
	apihandler.GinEngine.Any("/payment_webhooks", paymentwebhooks.Handler)

	apihandler.StartLambda()
}
