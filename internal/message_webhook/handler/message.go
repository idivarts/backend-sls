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
	err := msg.conversationData.Get(msg.IGSID)
	if err != nil || msg.conversationData.IGSID == "" {
		// return err
		// This is where I would need to create a new instance
		log.Println("Error Finding IGSID")
		msg.conversationData, err = msg.createMessageThread()
		if err != nil {
			return err
		}
		// return nil
	}
	if msg.Message != nil {
		err = msg.handleMessageThreadOperation()
	} else if msg.Read != nil {
		err = msg.handleReadOperation()
	}
	return err
}

func (msg IGMessagehandler) handleMessageThreadOperation() error {
	log.Println("Handling Message Send Logic", msg.conversationData.IGSID, msg.conversationData.ThreadID, msg.Message.Text)
	_, err := openai.SendMessage(msg.conversationData.ThreadID, msg.Message.Text, false)
	if err != nil {
		return err
	}

	_, err = msg.conversationData.UpdateLastMID(msg.Message.Mid)
	if err != nil {
		return err
	}

	// TODO Write code to time the send of message
	log.Println("Timing the Duration for the next message")

	// Generate a random integer between 0 and 10
	sendTimeDuration, err := timehandler.CalculateMessageDelay(msg.conversationData) // Generates a random integer in [0, 11)
	if err != nil {
		return err
	}

	delayedsqs.StopExecutions(msg.conversationData.MessageQueue)
	delayedsqs.StopExecutions(msg.conversationData.ReminderQueue)

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
		execArn, err := delayedsqs.Send(string(jData), int64(REMINDER_SECONDS+(*sendTimeDuration)))
		if err != nil {
			return err
		}
		msg.conversationData.ReminderQueue = execArn.ExecutionArn
	}
	_, err = msg.conversationData.Insert()
	if err != nil {
		return err
	}

	log.Println("Message sent to the queue after", *sendTimeDuration)

	return nil
}
