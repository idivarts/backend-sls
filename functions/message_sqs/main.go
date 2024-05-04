package main

import (
	messagesqs "github.com/TrendsHub/th-backend/internal/message_sqs"
	"github.com/aws/aws-lambda-go/lambda"
)

func main() {
	lambda.Start(messagesqs.Handler)
}
