package mwh_handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	sqsevents "github.com/idivarts/backend-sls/internal/message_sqs/events"
	"github.com/idivarts/backend-sls/internal/models"
	timehandler "github.com/idivarts/backend-sls/internal/time_handler"
	delayedsqs "github.com/idivarts/backend-sls/pkg/delayed_sqs"
	instainterfaces "github.com/idivarts/backend-sls/pkg/interfaces/instaInterfaces"
	"github.com/idivarts/backend-sls/pkg/myopenai"
)

type IGMessagehandler struct {
	ConversationID   string
	LeadID           string
	SourceID         string
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
	leadId := msg.LeadID

	// If the message is echo from the page itself, then the leadId should be the recipient
	if msg.SourceID == msg.LeadID {
		leadId = msg.Entry.Recipient.ID
	}

	// Get the source from the source Id and make sure that the source is active and linked with a campaign

	// Get conversation from only the specified campaign convesation. Please note, a conversation can exists with same id on mulitple campaigns
	err := msg.conversationData.GetByLead(leadId)
	if err != nil {
		// This is where I would need to create a new instance
		log.Println("Error Finding LeadId", err.Error())
		msg.conversationData = &models.Conversation{
			SourceID: msg.SourceID,
			LeadID:   leadId,
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

	if msg.SourceID != msg.LeadID ||
		//Checking last time bot processed the message was more than 2 minutes before the recorded time
		msg.conversationData.LastBotMessageTime < (msg.Entry.Timestamp-120000) {
		log.Println("Handling Message Send Logic", msg.conversationData.LeadID, msg.conversationData.ThreadID, msg.Message.Text)

		var richContent []myopenai.ContentRequest = nil

		if msg.Message.Attachments != nil && len(*msg.Message.Attachments) > 0 {
			log.Println("Handling Attachments. Setting status and exiting")

			richContent = []myopenai.ContentRequest{}
			for _, v := range *msg.Message.Attachments {
				if v.Type == "image" {
					f, err := myopenai.UploadImage(v.Payload.URL)
					if err != nil {
						return err
					}
					richContent = append(richContent, myopenai.ContentRequest{
						Type:      myopenai.ImageContentType,
						ImageFile: myopenai.ImageFile{FileID: f.ID},
					})
				}
			}

			if msg.Message.Text != "" {
				richContent = append(richContent, myopenai.ContentRequest{
					Type: myopenai.Text,
					Text: msg.Message.Text,
				})
			}
		}

		if msg.Message.Text == "" && len(richContent) == 0 {
			log.Println("Both Message and Rich Content is empty")
			return fmt.Errorf("error with webhook data : %s", "Both Message and Rich content is empty")
		}

		_, err := myopenai.SendMessage(msg.conversationData.ThreadID, msg.Message.Text, richContent, msg.SourceID == msg.LeadID)
		if err != nil {
			return err
		}
	}

	if msg.SourceID != msg.LeadID {
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
			LeadID:   msg.conversationData.LeadID,
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

	if msg.SourceID == msg.LeadID {
		log.Println("Handling Reminder Logics", msg.conversationData.LeadID, msg.conversationData.ThreadID, msg.Message.Text)
		delayedsqs.StopExecutions(msg.conversationData.ReminderQueue)
		if msg.conversationData.ReminderQueue == nil {
			msg.conversationData.ReminderCount = msg.conversationData.ReminderCount + 1
		} else {
			msg.conversationData.ReminderCount = 0
		}
		if msg.conversationData.CurrentPhase < 5 {
			event := sqsevents.ConversationEvent{
				LeadID:   msg.conversationData.LeadID,
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
			log.Println("Reminder Set after", REMINDER_SECONDS, msg.LeadID)
		} else {
			log.Println("Ignoring reminder", msg.LeadID)
		}
	}
	_, err := msg.conversationData.Insert()
	if err != nil {
		return err
	}
	return nil
}
