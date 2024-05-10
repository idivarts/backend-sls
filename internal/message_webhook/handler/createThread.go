package mwh_handler

import (
	"errors"
	"log"

	"github.com/TrendsHub/th-backend/internal/models"
	"github.com/TrendsHub/th-backend/pkg/messenger"
	"github.com/TrendsHub/th-backend/pkg/openai"
)

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

	lastMid := ""
	conv := convIds.Data[0]
	for i := len(conv.Messages.Data) - 1; i >= 1; i-- {
		entry := &conv.Messages.Data[i]
		log.Println("Sending Message", threadId, entry.Message, msg.IGSID != entry.From.ID)
		_, err = openai.SendMessage(threadId, entry.Message, msg.IGSID != entry.From.ID)
		if err != nil {
			return nil, err
		}
		lastMid = entry.ID
	}

	log.Println("Inserting the Conversation Model", msg.IGSID, threadId)
	data := &models.Conversation{
		IGSID:    msg.IGSID,
		ThreadID: threadId,
		LastMID:  lastMid,
	}
	_, err = (data).Insert()
	if err != nil {
		return nil, err
	}

	// openai.SendMessage(threadId, msg.Message.Text, false)
	return data, nil
}
