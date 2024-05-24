package mwh_handler

import (
	"fmt"
	"log"

	"github.com/TrendsHub/th-backend/internal/models"
	"github.com/TrendsHub/th-backend/pkg/messenger"
	"github.com/TrendsHub/th-backend/pkg/openai"
)

func (msg *IGMessagehandler) _fetchMessages(convId string, after *string, pageAccessToken string) []messenger.Message {
	if after != nil && *after == "" {
		return []messenger.Message{}
	}
	messages := []messenger.Message{}
	aStr := ""
	if after != nil {
		aStr = *after
	}
	data, err := messenger.GetMessagesWithPagination(convId, aStr, 20, pageAccessToken)
	if err != nil {
		return []messenger.Message{}
	}
	messages = append(messages, data.Data...)
	messages = append(messages, msg._fetchMessages(convId, &data.Paging.Cursors.After, pageAccessToken)...)
	return messages
}

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
		return nil, fmt.Errorf("error : %s", "Cant find any conversation with this userid")
	}

	lastMid := ""
	conv := convIds.Data[0]

	messages := msg._fetchMessages(conv.ID, nil, pData.AccessToken)

	lastindex := 1
	if includeLastMessage {
		lastindex = 0
	}
	for i := len(messages) - 1; i >= lastindex; i-- {
		entry := &messages[i]
		message := entry.Message

		var richContent []openai.ContentRequest = nil
		if entry.Attachments != nil && len(entry.Attachments.Data) > 0 {
			log.Println("Handling Attachments. Setting status and exiting")

			richContent = []openai.ContentRequest{}
			for _, v := range entry.Attachments.Data {
				if v.ImageData != nil {
					f, err := openai.UploadImage(v.ImageData.URL)
					if err != nil {
						log.Println("File upload error", err.Error())
						// return nil, err
					} else {
						richContent = append(richContent, openai.ContentRequest{
							Type:      openai.ImageContentType,
							ImageFile: openai.ImageFile{FileID: f.ID},
						})
					}
				}
			}

			if message != "" {
				richContent = append(richContent, openai.ContentRequest{
					Type: openai.Text,
					Text: message,
				})
			}
		}

		if message == "" && len(richContent) == 0 {
			log.Println("Both Message and Rich Content is empty")
			message = "[Attached Video/Link/Shares that cant be read by Chat Assistant]"
		}
		log.Println("Sending Message", threadId, message, msg.PageID == entry.From.ID)
		_, err = openai.SendMessage(threadId, message, richContent, msg.PageID == entry.From.ID)
		if err != nil {
			log.Println("Something went wrong while inseting the message", err.Error())
			// return nil, err
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
