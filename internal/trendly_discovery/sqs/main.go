package trendly_discovery_sqs

import (
	"context"
	"fmt"

	"github.com/aws/aws-lambda-go/events"
)

func Handler(ctx context.Context, sqsEvent events.SQSEvent) error {
	for _, message := range sqsEvent.Records {
		fmt.Printf("Received message ID %s from source %s\n", message.MessageId, message.EventSource)

		socialId := message.Body
		MoveImagesToS3(socialId)
	}
	return nil
}
