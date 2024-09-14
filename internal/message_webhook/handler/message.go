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
	"github.com/TrendsHub/th-backend/pkg/messenger"
	"github.com/TrendsHub/th-backend/pkg/openai"
)

type IGMessagehandler struct {
	// ConversationID   string
	LeadID           string
	SourceID         string
	Entry            *instainterfaces.Messaging
	Message          *instainterfaces.Message
	Read             *instainterfaces.Read
	conversationData *models.Conversation
}

func (msg IGMessagehandler) HandleMessage() error {
	if msg.Message == nil && msg.Read == nil {
		return errors.New("message or read body is empty")
	}

	log.Println("Getting the conversation from dynamoDB")
	msg.conversationData = &models.Conversation{}
	leadId := msg.LeadID

	// If the message is echo from the page itself, then the leadId should be the recipient
	if msg.SourceID == msg.LeadID {
		leadId = msg.Entry.Recipient.ID
	}

	// Get conversation from only the specified campaign convesation. Please note, a conversation can exists with same id on mulitple campaigns
	err := msg.conversationData.GetByLead(leadId)
	if err != nil {
		return err
	}

	organizationID := msg.conversationData.OrganizationID

	// Get the source from the source Id and make sure that the source is active and linked with a campaign
	source := models.Source{}
	err = source.Get(organizationID, msg.SourceID)
	if err != nil {
		return err
	}

	lead := models.Lead{}
	err = lead.Get(organizationID, msg.LeadID)
	if err != nil {
		log.Println("Lead not found. Creating a new lead", err.Error())
		sP := &models.SourcePrivate{}
		err = sP.Get(organizationID, source.ID)
		if err != nil {
			return err
		}
		var uProfile *messenger.UserProfile
		if source.SourceType == models.Instagram {
			uProfile, err = messenger.GetUser(msg.LeadID, *sP.AccessToken)
			if err != nil {
				return err
			}
		}
		lead = models.Lead{
			ID:          msg.LeadID,
			SourceType:  source.SourceType,
			SourceID:    source.ID,
			UserProfile: uProfile,
			Status:      1,
			CreatedAt:   time.Now().Unix(),
			UpdatedAt:   time.Now().Unix(),
		}
		_, err = lead.Insert(organizationID)
		if err != nil {
			return err
		}
	}

	if lead.Status != 1 || source.Status != 1 {
		return fmt.Errorf("lead or source is not active %d %d", lead.Status, source.Status)
	}

	if msg.conversationData.ThreadID == "" || msg.Message.IsDeleted {
		msg.conversationData, err = msg.createMessageThread(msg.Message.IsDeleted)
		if err != nil {
			return err
		}
		if msg.Message.IsDeleted {
			return nil
		}
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

		_, err := openai.SendMessage(msg.conversationData.ThreadID, msg.Message.Text, richContent, msg.SourceID == msg.LeadID)
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
