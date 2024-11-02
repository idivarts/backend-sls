package main

import (
	"github.com/aws/aws-lambda-go/lambda"
	messagesqs "github.com/idivarts/backend-sls/internal/message_sqs"
)

func main() {
	lambda.Start(messagesqs.Handler)
}
