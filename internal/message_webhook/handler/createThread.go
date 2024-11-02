package mwh_handler

import (
	"log"

	"github.com/idivarts/backend-sls/internal/models"
)

func (msg *IGMessagehandler) createMessageThread(includeLastMessage bool) (*models.Conversation, error) {
	log.Println("Creating new Message Thread")
	err := msg.conversationData.CreateThread(includeLastMessage)
	if err != nil {
		return nil, err
	}
	return msg.conversationData, nil
}
