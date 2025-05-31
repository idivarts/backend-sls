package streamsqs

import (
	"context"
	"fmt"

	"github.com/aws/aws-lambda-go/events"
)

func Handler(ctx context.Context, sqsEvent events.SQSEvent) error {
	for _, message := range sqsEvent.Records {
		fmt.Printf("The message %s for event source %s = %s \n", message.MessageId, message.EventSource, message.Body)
		// err := sendMessage(message.Body)
		// if err != nil {
		// 	log.Println(err.Error())
		// }
	}
	return nil
}
