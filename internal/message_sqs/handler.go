package messagesqs

import (
	"context"
	"encoding/json"
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

	if conv.IGSID == "" || conv.Action == "" {
		return fmt.Errorf("error - empty field - %s", "IGSID or Action")
	}

	if conv.Action == sqsevents.SEND_MESSAGE {
		return eventhandling.WaitAndSend(conv)
	} else if conv.Action == sqsevents.REMINDER {
		return eventhandling.SendReminder(conv)
	} else if conv.Action == sqsevents.RUN_OPENAI {
		return eventhandling.RunOpenAI(conv, "")
	} else if conv.Action == sqsevents.CREATE_THREAD || conv.Action == sqsevents.CREATE_OR_UPDATE_THREAD {
		return eventhandling.CreateOrUpdateThread(conv)
	} else if conv.Action == sqsevents.INSTA_SEND {
		return eventhandling.InstaSend(conv)
	}
	return nil
}
