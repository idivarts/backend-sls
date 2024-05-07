package eventhandling

import (
	"encoding/json"
	"log"

	sqsevents "github.com/TrendsHub/th-backend/internal/message_sqs/events"
	"github.com/TrendsHub/th-backend/pkg/messenger"
	"github.com/TrendsHub/th-backend/pkg/openai"
	sqshandler "github.com/TrendsHub/th-backend/pkg/sqs_handler"
)

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
