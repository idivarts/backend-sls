package main

import (
	"github.com/aws/aws-lambda-go/lambda"
	trendly_discovery_sqs "github.com/idivarts/backend-sls/internal/trendly_discovery/sqs"
)

func main() {
	lambda.Start(trendly_discovery_sqs.Handler)
}
