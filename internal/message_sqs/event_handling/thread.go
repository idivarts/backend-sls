package eventhandling

import (
	"log"

	sqsevents "github.com/TrendsHub/th-backend/internal/message_sqs/events"
	"github.com/TrendsHub/th-backend/internal/models"
)

func CreateOrUpdateThread(ev *sqsevents.ConversationEvent) error {
	igsid := ev.IGSID
	pageId := ev.RunID
	conv := &models.Conversation{}
	err := conv.Get(igsid)
	run := true
	if err != nil {
		conv = &models.Conversation{
			PageID: pageId,
			IGSID:  igsid,
		}
	} else {
		if ev.Action != sqsevents.CREATE_OR_UPDATE_THREAD {
			run = false
		}
	}

	if run {
		err = conv.CreateThread(true)
		if err != nil {
			log.Println("Errorr Creating Thread", err.Error())
			return err
		}
	}

	return nil
}
