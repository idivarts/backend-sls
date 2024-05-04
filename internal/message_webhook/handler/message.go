package mwh_handler

import (
	"errors"
	"log"

	"github.com/TrendsHub/th-backend/internal/models"
	instainterfaces "github.com/TrendsHub/th-backend/pkg/interfaces/instaInterfaces"
	"github.com/TrendsHub/th-backend/pkg/messenger"
	"github.com/TrendsHub/th-backend/pkg/openai"
	sqshandler "github.com/TrendsHub/th-backend/pkg/sqs_handler"
)

type IGMessagehandler struct {
	ConversationID string
	IGSID          string
	Message        *instainterfaces.Message
}

func (msg IGMessagehandler) HandleMessage() error {
	if msg.Message == nil {
		return errors.New("Message Body is empty")
	}

	log.Println("Getting the conversation from dynamoDB")
	var data *models.Conversation
	err := data.Get(msg.IGSID)
	if err != nil {
		// return err
		// This is where I would need to create a new instance
		data, err = msg.createMessageThread()
		if err != nil {
			return err
		}
		// return nil
	}
	err = msg.handleMessageThreadOperation(data.ThreadID)
	return err
}
func (msg IGMessagehandler) handleMessageThreadOperation(threadId string) error {
	log.Println("Handling Message Send Logic")
	err := openai.SendMessage(threadId, msg.Message.Text, false)
	if err != nil {
		return err
	}

	// TODO Write code to time the send of message
	log.Println("Timing the Duration for the next message")
	sendTimeDuration := 10

	err = sqshandler.SendToMessageQueue(threadId, int64(sendTimeDuration))
	if err != nil {
		return err
	}

	log.Println("Message sent to the queue after", sendTimeDuration)

	return nil
}
func (msg IGMessagehandler) createMessageThread() (*models.Conversation, error) {
	log.Println("Creating new Message Thread")
	thread, err := openai.CreateThread()
	if err != nil {
		return nil, err
	}
	threadId := thread.ID

	log.Println("Getting all conversations for this user")
	convIds, err := messenger.GetConversationsByUserId(msg.IGSID)
	if err != nil {
		return nil, err
	}

	if len(convIds.Data) == 0 {
		return nil, errors.New("Cant find any conversation with this userid")
	}

	conv := convIds.Data[0]
	for i := len(conv.Messages.Data) - 1; i >= 1; i-- {
		entry := &conv.Messages.Data[i]
		log.Println("Sending Message", threadId, entry.Message, msg.IGSID != entry.From.ID)
		err = openai.SendMessage(threadId, entry.Message, msg.IGSID != entry.From.ID)
		if err != nil {
			return nil, err
		}
	}

	log.Println("Inserting the Conversation Model", msg.IGSID, threadId)
	data := &models.Conversation{
		IGSID:    msg.IGSID,
		ThreadID: threadId,
	}
	_, err = (data).Insert()
	if err != nil {
		return nil, err
	}

	// openai.SendMessage(threadId, msg.Message.Text, false)
	return data, nil
}
