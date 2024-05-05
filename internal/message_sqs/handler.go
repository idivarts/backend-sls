package messagesqs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/TrendsHub/th-backend/pkg/messenger"
	"github.com/TrendsHub/th-backend/pkg/openai"
	sqshandler "github.com/TrendsHub/th-backend/pkg/sqs_handler"
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

type ConversationEvent struct {
	Action   string `json:"action"`
	IGSID    string `json:"igsid"`
	ThreadID string `json:"threadId"`
}

func sendMessage(message string) error {
	conv := &ConversationEvent{}
	err := json.Unmarshal([]byte(message), conv)
	if err != nil {
		return err
	}

	if conv.IGSID == "" || conv.ThreadID == "" {
		return errors.New("Malformed Input")
	}

	if conv.Action == "sendMessage" {
		return WaitAndSend(conv)
		// return nil
	}
	log.Println("Starting Run")
	err = openai.StartRun(conv.ThreadID, openai.ArjunAssistant)
	if err != nil {
		return err
	}
	// go waitAndSend(conv)
	log.Println("Waiting 5 second before sending message")
	conv.Action = "sendMessage"
	b, err := json.Marshal(&conv)
	if err != nil {
		return err
	}
	log.Println("Sending wait message", string(b))
	sqshandler.SendToMessageQueue(string(b), 5)

	return nil
}
func WaitAndSend(conv *ConversationEvent) error {
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
