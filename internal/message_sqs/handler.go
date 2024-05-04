package messagesqs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/TrendsHub/th-backend/internal/models"
	"github.com/TrendsHub/th-backend/pkg/messenger"
	"github.com/TrendsHub/th-backend/pkg/openai"
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
	conv := &models.Conversation{}
	err := json.Unmarshal([]byte(message), conv)
	if err != nil {
		return err
	}

	if conv.IGSID == "" || conv.ThreadID == "" {
		return errors.New("Malformed Input")
	}

	log.Println("Starting Run")
	err = openai.StartRun(conv.ThreadID, openai.ArjunAssistant)
	if err != nil {
		return err
	}
	time.Sleep(5 * time.Second)

	log.Println("Getting messaged from thread", conv.ThreadID)
	msgs, err := openai.GetMessages(conv.ThreadID)
	if err != nil {
		return err
	}
	log.Println("Message received", len(msgs.Data[0].Content), msgs.Data[0].Content[0].Text.Value)
	aMsg := msgs.Data[0].Content[0].Text

	log.Println("Sending Message", conv.IGSID, aMsg.Value, msgs.Data[0].ID)
	messenger.SendTextMessage(conv.IGSID, aMsg.Value)

	return nil
}
