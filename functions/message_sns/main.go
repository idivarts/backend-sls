package main

import (
	"context"
	"fmt"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

func handleSnsEvent(ctx context.Context, snsEvent events.SNSEvent) (string, error) {
	for _, record := range snsEvent.Records {
		snsRecord := record.SNS
		fmt.Printf("Message ID: %s\n", snsRecord.MessageID)
		fmt.Printf("Subject: %s\n", snsRecord.Subject)
		fmt.Printf("Message: %s\n", snsRecord.Message)
	}

	return "Success", nil
}

func main() {
	lambda.Start(handleSnsEvent)
}
