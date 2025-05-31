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
			for channelID, reminderData := range webhook.Channels {
				fmt.Printf("Channel ID: %s\n", channelID)
				fmt.Printf("Messages: %d\n", len(reminderData.Messages))
				for _, message := range reminderData.Messages {
					fmt.Printf("Message ID: %s, Text: %s\n", message.ID, message.Text)
				}
			}
		} else {
			fmt.Printf("Unhandled event type: %s\n", webhook.Type)
		}
	}
	return nil
}
