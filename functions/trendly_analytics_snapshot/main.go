// Package main implements a scheduled Lambda that snapshots daily top-line
// social analytics for every brand's connected Meta accounts.
//
// Runs once a day via EventBridge cron. For each brand it fetches fresh
// Instagram/Facebook insights and writes one AnalyticsSnapshot per account
// (followers point-in-time; reach/views/engagement as the latest daily value).
// Accumulated snapshots power historical trend graphs that the live Meta API
// cannot reproduce, while the live dashboard remains storage-light.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/idivarts/backend-sls/internal/trendlyapis/analytics"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

func main() {
	lambda.Start(handler)
}

func handler(ctx context.Context) error {
	date := time.Now().UTC().Format("2006-01-02")
	log.Printf("analytics_snapshot: starting for %s", date)

	brandDocs, err := firestoredb.Client.Collection("brands").Documents(ctx).GetAll()
	if err != nil {
		return fmt.Errorf("analytics_snapshot: failed to list brands: %w", err)
	}

	totalWritten, totalFailed := 0, 0
	for _, b := range brandDocs {
		w, f := analytics.SnapshotBrand(b.Ref.ID, date)
		totalWritten += w
		totalFailed += f
	}

	log.Printf("analytics_snapshot: done — written=%d failed=%d brands=%d",
		totalWritten, totalFailed, len(brandDocs))
	return nil
}
