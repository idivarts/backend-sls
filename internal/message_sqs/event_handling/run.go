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
	err := cData.Get(conv.IGSID)
	if err != nil {
		return err
	}

	pData := &models.Page{}
	err = pData.Get(cData.PageID)
	if err != nil || pData.PageID == "" {
		return err
	}

	if cData.LastMID != conv.MID {
		log.Println("This message is old.. Waiting for new message", cData.LastMID, conv.MID)
		return nil
	}

	if !cData.IsProfileFetched {
		uProfile, err := messenger.GetUser(cData.IGSID, pData.AccessToken)
		if err != nil {
			return err
		}
		if additionalInstruction != "" {
			additionalInstruction = fmt.Sprintf("%s\n-------------\n%s", additionalInstruction, uProfile.GenerateUserDescription())
		} else {
			additionalInstruction = uProfile.GenerateUserDescription()
		}

		cData.IsProfileFetched = true
		cData.UserProfile = uProfile
		cData.Insert()
		// cData.UpdateProfileFetched()
	}
	log.Println("Starting Run")
	rObj, err := openai.StartRun(conv.ThreadID, openai.AssistantID(pData.AssistantID), additionalInstruction, "")
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
