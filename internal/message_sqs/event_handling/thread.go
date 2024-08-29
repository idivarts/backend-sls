package eventhandling

import (
	"log"

	sqsevents "github.com/TrendsHub/th-backend/internal/message_sqs/events"
	"github.com/TrendsHub/th-backend/internal/models"
)

func CreateOrUpdateThread(ev *sqsevents.ConversationEvent) error {
	igsid := ev.IGSID
	pageId := ev.PageID
	conv := &models.Conversation{}
	err := conv.GetByLead(igsid)
	run := true
	if err != nil {
		conv = &models.Conversation{
			SourceID: pageId,
			LeadID:   igsid,
		}
	} else {
		if ev.Action != sqsevents.CREATE_OR_UPDATE_THREAD {
			log.Println("Wont be updating this thread", igsid)
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
	if ev.Action == sqsevents.CREATE_THREAD || ev.Action == sqsevents.CREATE_OR_UPDATE_THREAD {
		pData := &models.Source{}
		err := pData.Get(conv.OrganizationID, pageId)
		if err != nil {
			return err
		}
		// TODO: Write code to update the lead table
		// user, err := messenger.GetUser(igsid, *pData.AccessToken)
		// if err != nil {
		// 	return err
		// }
		// conv.UserProfile = user
		// _, err = conv.Insert()
		// if err != nil {
		// 	return err
		// }
	}

	return nil
}
