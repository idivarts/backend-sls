package eventhandling

import (
	"encoding/json"
	"log"

	sqsevents "github.com/TrendsHub/th-backend/internal/message_sqs/events"
	"github.com/TrendsHub/th-backend/internal/models"
	"github.com/TrendsHub/th-backend/pkg/messenger"
	"github.com/TrendsHub/th-backend/pkg/openai"
	sqshandler "github.com/TrendsHub/th-backend/pkg/sqs_handler"
)

func RunOpenAI(conv *sqsevents.ConversationEvent) error {
	cData := &models.Conversation{}
	err := cData.Get(conv.IGSID)
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
		cData.UpdateProfileFetched()
	}
	log.Println("Starting Run")
	rObj, err := openai.StartRun(conv.ThreadID, openai.ArjunAssistant, additionalInstruction, "")
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
