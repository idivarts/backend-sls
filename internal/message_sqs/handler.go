package messagesqs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	sqsevents "github.com/TrendsHub/th-backend/internal/message_sqs/events"
	"github.com/TrendsHub/th-backend/internal/models"
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

func sendMessage(message string) error {
	conv := &sqsevents.ConversationEvent{}
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

	cData := &models.Conversation{}
	err = cData.Get(conv.IGSID)
	if err != nil || cData.IGSID == "" {
		return err
	}

	if cData.LastMID != conv.MID {
		log.Println("This message is old.. Waiting for new message", cData.LastMID, conv.MID)
		return nil
	}

	additionalInstruction := ""
	if !cData.IsProfileFetched {
		uProfile, err := messenger.GetUser(cData.IGSID)
		if err != nil {
			return err
		}
		additionalInstruction = uProfile.GenerateUserDescription()
	}
	log.Println("Starting Run")
	rObj, err := openai.StartRun(conv.ThreadID, openai.ArjunAssistant, additionalInstruction)
	if err != nil {
		return err
	}
	// go waitAndSend(conv)
	log.Println("Waiting 5 second before sending message")
	conv.Action = "sendMessage"
	conv.RunID = rObj.ID
	b, err := json.Marshal(&conv)
	if err != nil {
		return err
	}
	log.Println("Sending wait message", string(b))
	sqshandler.SendToMessageQueue(string(b), 5)

	return nil
}
func WaitAndSend(conv *sqsevents.ConversationEvent) error {
	log.Println("Getting messaged from thread", conv.ThreadID)
	msgs, err := openai.GetMessages(conv.ThreadID, 1, conv.RunID)
	if err != nil {
		return err
	}
	log.Println("Message received", len(msgs.Data[0].Content), msgs.Data[0].Content[0].Text.Value)

	for _, v := range msgs.Data {
		if v.RunID == conv.RunID {
			aMsg := v.Content[0].Text
			log.Println("Sending Message", conv.IGSID, aMsg.Value, v.ID)
			messenger.SendTextMessage(conv.IGSID, aMsg.Value)

			return nil
		}
	}

	log.Println("Message not found - Waiting 1 second")
	b, err := json.Marshal(conv)
	if err != nil {
		return err
	}
	log.Println("Sending wait message", string(b))
	sqshandler.SendToMessageQueue(string(b), 1)
	return nil

}
