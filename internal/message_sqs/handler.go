package messagesqs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	eventhandling "github.com/TrendsHub/th-backend/internal/message_sqs/event_handling"
	sqsevents "github.com/TrendsHub/th-backend/internal/message_sqs/events"
	"github.com/aws/aws-lambda-go/events"
)

func Handler(ctx context.Context, sqsEvent events.SQSEvent) error {
	for _, message := range sqsEvent.Records {
		fmt.Printf("The message %s for event source %s = %s \n", message.MessageId, message.EventSource, message.Body)
		err := sendMessage(message.Body)
		if err != nil {
			log.Println(err.Error())
		}
	}
	return nil
}

func sendMessage(message string) error {
	conv := &sqsevents.ConversationEvent{}
	err := json.Unmarshal([]byte(message), conv)
	if err != nil {
		return err
	}

	if conv.IGSID == "" || conv.ThreadID == "" {
		return errors.New("Malformed Input")
	}

	if conv.Action == sqsevents.SEND_MESSAGE {
		return eventhandling.WaitAndSend(conv)
	} else {
		return eventhandling.RunOpenAI(conv)
	}

}
