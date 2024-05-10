package eventhandling

import (
	"encoding/json"
	"log"

	sqsevents "github.com/TrendsHub/th-backend/internal/message_sqs/events"
	"github.com/TrendsHub/th-backend/internal/models"
	"github.com/TrendsHub/th-backend/pkg/openai"
	sqshandler "github.com/TrendsHub/th-backend/pkg/sqs_handler"
)

func SendReminder(conv *sqsevents.ConversationEvent) error {
	cData := &models.Conversation{}
	err := cData.Get(conv.IGSID)
	if err != nil || cData.IGSID == "" {
		return err
	}

	cData.ReminderQueue = nil

	_, err = cData.Insert()
	if err != nil {
		return err
	}

	additionalInstruction := "The user has not replied in 6 hours. Remind them gently!"
	log.Println("Starting Reminder Run")
	rObj, err := openai.StartRun(conv.ThreadID, openai.ArjunAssistant, additionalInstruction, "")
	if err != nil {
		return err
	}
	// go waitAndSend(conv)
	log.Println("Waiting 5 second before sending reminder message")
	conv.Action = sqsevents.SEND_MESSAGE
	conv.RunID = rObj.ID
	b, err := json.Marshal(&conv)
	if err != nil {
		return err
	}
	log.Println("Sending reminder wait message", string(b))
	sqshandler.SendToMessageQueue(string(b), 5)

	return nil
}
