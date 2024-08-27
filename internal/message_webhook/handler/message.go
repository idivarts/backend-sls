package mwh_handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

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
	if err != nil {
		// This is where I would need to create a new instance
		log.Println("Error Finding IGSID", err.Error())
		msg.conversationData = &models.Conversation{
			SourceID: msg.PageID,
			IGSID:    convId,
		}
		msg.conversationData, err = msg.createMessageThread(false)
		if err != nil {
			return err
		}
	} else if msg.Message.IsDeleted {
		log.Println("Deleting and creating a brand new thread")
		msg.conversationData, err = msg.createMessageThread(true)
		if err != nil {
			return err
		}
		return nil
	}
	// if msg.Message != nil {
	err = msg.handleMessageThreadOperation()
	if err != nil {
		delayedsqs.StopExecutions(msg.conversationData.MessageQueue)
		delayedsqs.StopExecutions(msg.conversationData.ReminderQueue)

		msg.conversationData.CurrentPhase = 7
		_, err := msg.conversationData.Insert()
		if err != nil {
			return err
		}
		return nil
	}
	return err
}

func (msg IGMessagehandler) handleMessageThreadOperation() error {

	msg.conversationData.LastMID = msg.Message.Mid

	if msg.conversationData.CurrentPhase >= 6 || msg.conversationData.Status == 0 {
		log.Println("Processing is paused for this message thread", msg.conversationData.CurrentPhase, msg.conversationData.Status)
		return nil
	}

	if msg.PageID != msg.IGSID ||
		//Checking last time bot processed the message was more than 2 minutes before the recorded time
		msg.conversationData.LastBotMessageTime < (msg.Entry.Timestamp-120000) {
		log.Println("Handling Message Send Logic", msg.conversationData.IGSID, msg.conversationData.ThreadID, msg.Message.Text)

		var richContent []openai.ContentRequest = nil

		if msg.Message.Attachments != nil && len(*msg.Message.Attachments) > 0 {
			log.Println("Handling Attachments. Setting status and exiting")

			richContent = []openai.ContentRequest{}
			for _, v := range *msg.Message.Attachments {
				if v.Type == "image" {
					f, err := openai.UploadImage(v.Payload.URL)
					if err != nil {
						return err
					}
					richContent = append(richContent, openai.ContentRequest{
						Type:      openai.ImageContentType,
						ImageFile: openai.ImageFile{FileID: f.ID},
					})
				}
			}

			if msg.Message.Text != "" {
				richContent = append(richContent, openai.ContentRequest{
					Type: openai.Text,
					Text: msg.Message.Text,
				})
			}
		}

		if msg.Message.Text == "" && len(richContent) == 0 {
			log.Println("Both Message and Rich Content is empty")
			return fmt.Errorf("error with webhook data : %s", "Both Message and Rich content is empty")
		}

		_, err := openai.SendMessage(msg.conversationData.ThreadID, msg.Message.Text, richContent, msg.PageID == msg.IGSID)
		if err != nil {
			return err
		}
	}

	if msg.PageID != msg.IGSID {
		delayedsqs.StopExecutions(msg.conversationData.MessageQueue)
		delayedsqs.StopExecutions(msg.conversationData.ReminderQueue)

		msg.conversationData.ReminderCount = 0
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
		nextMessageTime := time.Now().Unix() + int64(*sendTimeDuration)
		msg.conversationData.NextReminderTime = &nextMessageTime
		msg.conversationData.MessageQueue = execArn.ExecutionArn
		log.Println("Message sent to the queue after", *sendTimeDuration)
	}

	if msg.PageID == msg.IGSID {
		log.Println("Handling Reminder Logics", msg.conversationData.IGSID, msg.conversationData.ThreadID, msg.Message.Text)
		delayedsqs.StopExecutions(msg.conversationData.ReminderQueue)
		if msg.conversationData.ReminderQueue == nil {
			msg.conversationData.ReminderCount = msg.conversationData.ReminderCount + 1
		} else {
			msg.conversationData.ReminderCount = 0
		}
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
			sendTimeDuration := timehandler.CalculateRemiderDelay(msg.conversationData) // Generates a random integer in [0, 11)
			execArn, err := delayedsqs.Send(string(jData), int64(sendTimeDuration))
			if err != nil {
				return err
			}
			nextReminderTime := time.Now().Unix() + int64(sendTimeDuration)
			msg.conversationData.NextReminderTime = &nextReminderTime
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
