package main

import (
	"github.com/idivarts/backend-sls/internal/trendlyapis/revenuecat"
	apihandler "github.com/idivarts/backend-sls/pkg/api_handler"
)

func main() {
	// RevenueCat posts native IAP (Apple/Google) subscription + top-up events
	// here. Authenticated by the shared Authorization header (REVENUECAT_WEBHOOK_AUTH).
	apihandler.GinEngine.Any("/revenuecat/webhook", revenuecat.Handler)

	apihandler.StartLambda()
}
