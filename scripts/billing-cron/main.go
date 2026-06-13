package main

import (
	"context"
	"log"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/idivarts/backend-sls/internal/trendlyapis/billing"
)

// Monthly billing cron — runs on the 1st of each month: renews every org's token
// wallet to its plan allotment and advances the access-state machine (past_due →
// locked, invoice month re-lock). See the Credit ticket §5/§7 Phase 5.
func main() {
	lambda.Start(handler)
}

func handler(ctx context.Context) (string, error) {
	start := time.Now()
	log.Println("billing cron: start", start.UTC())
	if err := billing.RunMonthlyBilling(); err != nil {
		log.Println("billing cron: error", err)
		return "error", err
	}
	log.Println("billing cron: done in", time.Since(start))
	return "ok", nil
}
