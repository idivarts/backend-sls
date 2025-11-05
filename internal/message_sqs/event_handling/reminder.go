package eventhandling

import (
	"encoding/json"
	"fmt"
	"log"

	sqsevents "github.com/idivarts/backend-sls/internal/message_sqs/events"
	"github.com/idivarts/backend-sls/internal/models"
	"github.com/idivarts/backend-sls/pkg/myopenai"
	sqshandler "github.com/idivarts/backend-sls/pkg/sqs_handler"
)

func SendReminder(conv *sqsevents.ConversationEvent) error {
	cData := &models.Conversation{}
	err := cData.GetByLead(conv.LeadID)
	if err != nil {
		return err
	}

	campaign := &models.Campaign{}
	err = campaign.Get(cData.OrganizationID, cData.CampaignID)
	if err != nil {
		return err
	}

	pData := &models.Source{}
	err = pData.Get(cData.OrganizationID, cData.SourceID)
	if err != nil || pData.ID == "" {
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
	rObj, err := myopenai.StartRun(conv.ThreadID, myopenai.AssistantID(*campaign.AssistantID), additionalInstruction, string(myopenai.ChangePhaseFn))
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
