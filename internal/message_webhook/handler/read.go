package mwh_handler

import (
	"encoding/json"
	"errors"
	"log"

	sqsevents "github.com/TrendsHub/th-backend/internal/message_sqs/events"
	"github.com/TrendsHub/th-backend/internal/models"
	delayedsqs "github.com/TrendsHub/th-backend/pkg/delayed_sqs"
	"github.com/TrendsHub/th-backend/pkg/messenger"
)

func (msg IGMessagehandler) handleReadOperation() error {
	pData := models.Page{}
	err := pData.Get(msg.PageID)
	if err != nil || pData.PageID == "" {
		return err
	}

	oList, err := messenger.GetConversationMessages(msg.ConversationID, pData.AccessToken)
	if err != nil {
		return err
	}
	if len(oList.Messages.Data) == 0 {
		return errors.New("No Messages")
	}
	log.Println("Last Message Stat", oList.Messages.Data[0].From.ID, oList.ID)
	if oList.Messages.Data[0].From.ID == oList.ID && msg.conversationData.CurrentPhase < 5 {
		delayedsqs.StopExecutions(msg.conversationData.ReminderQueue)
		event := sqsevents.ConversationEvent{
			IGSID:    msg.conversationData.IGSID,
			ThreadID: msg.conversationData.ThreadID,
			MID:      msg.conversationData.LastMID,
			Action:   sqsevents.REMINDER,
		}
		jData, err := json.Marshal(event)
		if err != nil {
			return err
		}
		execArn, err := delayedsqs.Send(string(jData), int64(READ_REMINDER_SECONDS))
		if err != nil {
			return err
		}
		msg.conversationData.ReminderQueue = execArn.ExecutionArn

		_, err = msg.conversationData.Insert()
		if err != nil {
			return err
		}
	}
	return nil
}
