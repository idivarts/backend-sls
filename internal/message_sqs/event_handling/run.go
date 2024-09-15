package eventhandling

import (
	"encoding/json"
	"fmt"
	"log"

	sqsevents "github.com/TrendsHub/th-backend/internal/message_sqs/events"
	"github.com/TrendsHub/th-backend/internal/models"
	"github.com/TrendsHub/th-backend/pkg/messenger"
	"github.com/TrendsHub/th-backend/pkg/openai"
	sqshandler "github.com/TrendsHub/th-backend/pkg/sqs_handler"
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

	srcP := &models.SourcePrivate{}
	err = srcP.Get(cData.OrganizationID, cData.SourceID)
	if err != nil {
		return err
	}

	if cData.LastMID != conv.MID {
		log.Println("This message is old.. Waiting for new message", cData.LastMID, conv.MID)
		return nil
	}
	_, err = messenger.SendAction(cData.LeadID, messenger.MARK_SEEN, *srcP.AccessToken)
	if err != nil {
		log.Println("Error while send Action", err.Error())
	}

	if !cData.IsProfileFetched {
		uProfile, err := messenger.GetUser(cData.LeadID, *srcP.AccessToken)
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
	rObj, err := openai.StartRun(conv.ThreadID, openai.AssistantID(*campaign.AssistantID), additionalInstruction, string(openai.ChangePhaseFn))
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
