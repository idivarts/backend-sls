package main

import (
	"context"
	"log"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/idivarts/backend-sls/pkg/myquery"
	"github.com/idivarts/backend-sls/scripts/matchmaking/mm"
)

func main() {
	// Run as an AWS Lambda handler
	lambda.Start(handler)
}

func handler(ctx context.Context) (string, error) {
	start := time.Now().UnixMicro()
	log.Println("Lambda invocation start", start)

	str := myquery.Client.Project()
	log.Println("Client ProjectID", str)

	// query := myquery.Client.Query("SELECT * FROM `trendly-9ab99.matches.influencers` LIMIT 1000")
	// _, err := query.Read(context.Background())
	// if err != nil {
	// 	log.Fatalf("Failed to execute query: %v", err)
	// }
	// log.Println("Successful Connection")

	log.Println("Syncing Users")
	mm.SyncUsers(true)

	// log.Println("Syncing Brands")
	// mm.SyncBrands(false)

	log.Println("Sync Completed")

	log.Println("Lambda invocation end", time.Now().UnixMicro())
	return "ok", nil
}
