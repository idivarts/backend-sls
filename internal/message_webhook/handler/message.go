package mwh_handler

import (
	"encoding/json"
	"errors"
	"log"

	sqsevents "github.com/TrendsHub/th-backend/internal/message_sqs/events"
	"github.com/TrendsHub/th-backend/internal/models"
	timehandler "github.com/TrendsHub/th-backend/internal/time_handler"
	delayedsqs "github.com/TrendsHub/th-backend/pkg/delayed_sqs"
	instainterfaces "github.com/TrendsHub/th-backend/pkg/interfaces/instaInterfaces"
	"github.com/TrendsHub/th-backend/pkg/openai"
)

type IGMessagehandler struct {
	ConversationID   string
	IGSID            string
	PageID           string
	Entry            *instainterfaces.Messaging
	Message          *instainterfaces.Message
	Read             *instainterfaces.Read
	conversationData *models.Conversation
}

func (msg IGMessagehandler) HandleMessage() error {
	if msg.Message == nil && msg.Read == nil {
		return errors.New("Message and Read Body is empty")
	}

	log.Println("Getting the conversation from dynamoDB")
	msg.conversationData = &models.Conversation{}
	convId := msg.IGSID
	if msg.PageID == msg.IGSID {
		convId = msg.Entry.Recipient.ID
	}
	err := msg.conversationData.Get(convId)
	if msg.Message.IsDeleted {
		log.Println("Deleting and creating a brand new thread")
		msg.conversationData, err = msg.createMessageThread(convId, true)
		if err != nil {
			return err
		}
		return nil
	} else if err != nil || msg.conversationData.IGSID == "" {
		// This is where I would need to create a new instance
		log.Println("Error Finding IGSID")
		msg.conversationData, err = msg.createMessageThread(convId, false)
		if err != nil {
			return err
		}
	}
	// if msg.Message != nil {
	err = msg.handleMessageThreadOperation()
	// } else if msg.Read != nil {
	// 	err = msg.handleReadOperation()
	// }
	return err
}

func (msg IGMessagehandler) handleMessageThreadOperation() error {

	msg.conversationData.LastMID = msg.Message.Mid

	if msg.PageID != msg.IGSID ||
		//Checking last time bot processed the message was more than 20 seconds before the recorded time
		msg.conversationData.LastBotMessageTime < (msg.Entry.Timestamp-20000) {
		log.Println("Handling Message Send Logic", msg.conversationData.IGSID, msg.conversationData.ThreadID, msg.Message.Text)
		_, err := openai.SendMessage(msg.conversationData.ThreadID, msg.Message.Text, msg.PageID == msg.IGSID)
		if err != nil {
			return err
		}
	}

	if msg.PageID != msg.IGSID {
		delayedsqs.StopExecutions(msg.conversationData.MessageQueue)

		log.Println("Timing the Duration for the next message")
		// Generate a random integer between 0 and 10
		sendTimeDuration, err := timehandler.CalculateMessageDelay(msg.conversationData) // Generates a random integer in [0, 11)
		if err != nil {
			return err
		}

		event := sqsevents.ConversationEvent{
			IGSID:    msg.conversationData.IGSID,
			ThreadID: msg.conversationData.ThreadID,
			MID:      msg.conversationData.LastMID,
			Action:   sqsevents.RUN_OPENAI,
		}
		jData, err := json.Marshal(event)
		if err != nil {
			return err
		}
		execArn, err := delayedsqs.Send(string(jData), int64(*sendTimeDuration))
		if err != nil {
			return err
		}
		msg.conversationData.MessageQueue = execArn.ExecutionArn
		log.Println("Message sent to the queue after", *sendTimeDuration)
	}

	if msg.PageID == msg.IGSID {
		log.Println("Handling Reminder Logics", msg.conversationData.IGSID, msg.conversationData.ThreadID, msg.Message.Text)
		delayedsqs.StopExecutions(msg.conversationData.ReminderQueue)
		if msg.conversationData.CurrentPhase < 5 {
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
			execArn, err := delayedsqs.Send(string(jData), int64(REMINDER_SECONDS))
			if err != nil {
				return err
			}
			msg.conversationData.ReminderQueue = execArn.ExecutionArn
			log.Println("Reminder Set after", REMINDER_SECONDS, msg.IGSID)
		} else {
			log.Println("Ignoring reminder", msg.IGSID)
		}
	}
	_, err := msg.conversationData.Insert()
	if err != nil {
		return err
	}
	return nil
}
