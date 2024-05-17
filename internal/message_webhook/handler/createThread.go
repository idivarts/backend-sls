package mwh_handler

import (
	"errors"
	"log"

	"github.com/TrendsHub/th-backend/internal/models"
	"github.com/TrendsHub/th-backend/pkg/messenger"
	"github.com/TrendsHub/th-backend/pkg/openai"
)

func (msg *IGMessagehandler) createMessageThread(convId string, includeLastMessage bool) (*models.Conversation, error) {
	log.Println("Creating new Message Thread")
	pData := models.Page{}
	err := pData.Get(msg.PageID)
	if err != nil || pData.PageID == "" {
		return nil, err
	}

	thread, err := openai.CreateThread()
	if err != nil {
		return nil, err
	}
	threadId := thread.ID

	log.Println("Getting all conversations for this user")
	convIds, err := messenger.GetConversationsByUserId(convId, pData.AccessToken)
	if err != nil {
		return nil, err
	}

	if len(convIds.Data) == 0 {
		return nil, errors.New("Cant find any conversation with this userid")
	}

	lastMid := ""
	conv := convIds.Data[0]

	lastindex := 1
	if includeLastMessage {
		lastindex = 0
	}
	for i := len(conv.Messages.Data) - 1; i >= lastindex; i-- {
		entry := &conv.Messages.Data[i]
		log.Println("Sending Message", threadId, entry.Message, msg.PageID == entry.From.ID)
		_, err = openai.SendMessage(threadId, entry.Message, msg.PageID == entry.From.ID)
		if err != nil {
			return nil, err
		}
		lastMid = entry.ID
	}

	log.Println("Inserting the Conversation Model", convId, threadId)
	msg.conversationData.IGSID = convId
	msg.conversationData.PageID = msg.PageID
	msg.conversationData.ThreadID = threadId
	msg.conversationData.LastMID = lastMid
	// data := &models.Conversation{
	// 	IGSID:    convId,
	// 	ThreadID: threadId,
	// 	LastMID:  lastMid,
	// }
	_, err = (msg.conversationData).Insert()
	if err != nil {
		return nil, err
	}

	// openai.SendMessage(threadId, msg.Message.Text, false)
	return msg.conversationData, nil
}
