package eventhandling

import (
	"encoding/json"
	"fmt"
	"log"

	sqsevents "github.com/idivarts/backend-sls/internal/message_sqs/events"
	"github.com/idivarts/backend-sls/internal/models"
	"github.com/idivarts/backend-sls/pkg/messenger"
	"github.com/idivarts/backend-sls/pkg/myopenai"
	sqshandler "github.com/idivarts/backend-sls/pkg/sqs_handler"
)

func RunOpenAI(conv *sqsevents.ConversationEvent, additionalInstruction string) error {
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

	pData := &models.SourcePrivate{}
	err = pData.Get(cData.OrganizationID, cData.SourceID)
	if err != nil {
		return err
	}

	if cData.LastMID != conv.MID {
		log.Println("This message is old.. Waiting for new message", cData.LastMID, conv.MID)
		return nil
	}
	_, err = messenger.SendAction(cData.LeadID, messenger.MARK_SEEN, *pData.AccessToken)
	if err != nil {
		log.Println("Error while send Action", err.Error())
	}

	if !cData.IsProfileFetched {
		uProfile, err := messenger.GetUser(cData.LeadID, *pData.AccessToken)
		if err != nil {
			return err
		}
		if additionalInstruction != "" {
			additionalInstruction = fmt.Sprintf("%s\n-------------\n%s", additionalInstruction, uProfile.GenerateUserDescription())
		} else {
			additionalInstruction = uProfile.GenerateUserDescription()
		}

		cData.IsProfileFetched = true
		// TODO: Probably write code to update the lead table
		// cData.UserProfile = uProfile
		cData.Insert()
		// cData.UpdateProfileFetched()
	}
	log.Println("Starting Run")
	rObj, err := myopenai.StartRun(conv.ThreadID, myopenai.AssistantID(*campaign.AssistantID), additionalInstruction, string(myopenai.ChangePhaseFn))
	if err != nil {
		return err
	}
	// go waitAndSend(conv)
	log.Println("Waiting 5 second before sending message")
	conv.Action = sqsevents.SEND_MESSAGE
	conv.RunID = rObj.ID
	b, err := json.Marshal(&conv)
	if err != nil {
		return err
	}
	log.Println("Sending wait message", string(b))
	sqshandler.SendToMessageQueue(string(b), 5)

	return nil
}
