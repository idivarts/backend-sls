package streamsqs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-lambda-go/events"
)

func Handler(ctx context.Context, sqsEvent events.SQSEvent) error {
	for _, message := range sqsEvent.Records {
		fmt.Printf("Received message ID %s from source %s\n", message.MessageId, message.EventSource)

		var webhook StreamWebhook
		err := json.Unmarshal([]byte(message.Body), &webhook)
		if err != nil {
			fmt.Printf("Failed to parse message body: %v\n", err)
			continue
		}

		fmt.Printf("Parsed Stream Event:\nType: %s\nUser: %s\n",
			webhook.Type, webhook.User.Name)

		if webhook.Type == "user.unread_message_reminder" {
			HandleUnreadMessage(&webhook)
		} else {
			fmt.Printf("Unhandled event type: %s\n", webhook.Type)
		}
	}
	return nil
}
