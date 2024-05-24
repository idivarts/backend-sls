package eventhandling

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"

	sqsevents "github.com/TrendsHub/th-backend/internal/message_sqs/events"
	"github.com/TrendsHub/th-backend/internal/models"
	"github.com/TrendsHub/th-backend/pkg/openai"
	sqshandler "github.com/TrendsHub/th-backend/pkg/sqs_handler"
)

func SendReminder(conv *sqsevents.ConversationEvent) error {
	cData := &models.Conversation{}
	err := cData.Get(conv.IGSID)
	if err != nil {
		return err
	}
	if cData.IGSID == "" {
		return errors.New("Cant find this entry")
		// return
	}
	pData := &models.Page{}
	err = pData.Get(cData.PageID)
	if err != nil || pData.PageID == "" {
		return err
	}

	cData.ReminderQueue = nil

	_, err = cData.Insert()
	if err != nil {
		return err
	}

	timeData := "some time"
	if cData.ReminderCount > 0 {
		timeData = fmt.Sprintf("%d hours", (6 * (cData.ReminderCount + 1)))
	}
	additionalInstruction := fmt.Sprintf("The user has not replied in %s. Remind them gently. This is reminder %d", timeData, (cData.ReminderCount + 1))
	log.Println("Starting Reminder Run")
	rObj, err := openai.StartRun(conv.ThreadID, openai.AssistantID(pData.AssistantID), additionalInstruction, "")
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
