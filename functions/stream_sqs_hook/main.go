package main

import (
	"github.com/aws/aws-lambda-go/lambda"
	streamsqs "github.com/idivarts/backend-sls/internal/stream_sqs"
)

func main() {
	lambda.Start(streamsqs.Handler)
}
