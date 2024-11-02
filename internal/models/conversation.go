package models

import (
	"context"
	"fmt"
	"log"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"github.com/idivarts/backend-sls/pkg/messenger"
	"github.com/idivarts/backend-sls/pkg/openai"
	"google.golang.org/api/iterator"
)

type Conversation struct {
	LeadID string `json:"leadId"`

	OrganizationID     string            `json:"organizationId"`
	CampaignID         string            `json:"campaignId"`
	SourceID           string            `json:"sourceId"`
	ThreadID           string            `json:"threadId"`
	LastMID            string            `json:"lastMid"`
	LastBotMessageTime int64             `json:"lastBotMessageTime"`
	BotMessageCount    int               `json:"botMessageCount"`
	IsProfileFetched   bool              `json:"isProfileFetched"`
	Phases             []int             `json:"phases"`
	CurrentPhase       int               `json:"currentPhase"`
	Collectibles       map[string]string `json:"collectibles"`
	MessageQueue       *string           `json:"messageQueue,omitempty"`
	NextMessageTime    *int64            `json:"nextMessageTime,omitempty"`
	NextReminderTime   *int64            `json:"nextReminderTime,omitempty"`
	ReminderQueue      *string           `json:"reminderQueue,omitempty"`
	ReminderCount      int               `json:"reminderCount"`
	Status             int               `json:"status"`

	// Old fields that needs to be replaced or removed
	// IGSID       string                 `json:"igsid" dynamodbav:"igsid"`
	// UserProfile *messenger.UserProfile `json:"userProfile,omitempty" dynamodbav:"userProfile"`
	// Information openaifc.ChangePhase   `json:"information" dynamodbav:"information"`
}

func (conversation *Conversation) CreateThread(includeLastMessage bool) error {
	pData := SourcePrivate{}
	err := pData.Get(conversation.OrganizationID, conversation.SourceID)
	if err != nil {
		return err
	}

	thread, err := openai.CreateThread()
	if err != nil {
		return err
	}
	threadId := thread.ID

	log.Println("Getting all conversations for this user")
	convIds, err := messenger.GetConversationsByUserId(conversation.LeadID, *pData.AccessToken)
	if err != nil {
		return err
	}

	if len(convIds.Data) == 0 {
		return fmt.Errorf("error : %s", "Cant find any conversation with this userid")
	}

	lastMid := ""
	conv := convIds.Data[0]

	messages := messenger.FetchAllMessages(conv.ID, nil, *pData.AccessToken)

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
		log.Println("Sending Message", threadId, message, conversation.SourceID == entry.From.ID)
		_, err = openai.SendMessage(threadId, message, richContent, conversation.SourceID == entry.From.ID)
		if err != nil {
			log.Println("Something went wrong while inseting the message", err.Error())
			// return nil, err
		}
		lastMid = entry.ID
	}

	log.Println("Inserting the Conversation Model", conversation.LeadID, threadId)
	// conversation.IGSID = igsid
	// conversation.PageID = pageId
	conversation.ThreadID = threadId
	conversation.LastMID = lastMid
	conversation.Status = 1
	// data := &models.Conversation{
	// 	IGSID:    convId,
	// 	ThreadID: threadId,
	// 	LastMID:  lastMid,
	// }
	_, err = conversation.Insert()
	if err != nil {
		return err
	}

	return nil
}

func (c *Conversation) GetPath() (*string, error) {
	if c.OrganizationID == "" || c.CampaignID == "" {
		return nil, fmt.Errorf("Organzation(%s) of Campaign(%s) id cant be null", c.OrganizationID, c.CampaignID)
	}

	path := fmt.Sprintf("organizations/%s/campaigns/%s/conversations", c.OrganizationID, c.CampaignID)
	return &path, nil
}
func (c *Conversation) Insert() (*firestore.WriteResult, error) {
	path, err := c.GetPath()
	if err != nil {
		return nil, err
	}

	docRef := firestoredb.Client.Collection(*path).Doc(c.LeadID)
	res, err := docRef.Set(context.Background(), c)
	return res, err
}

func (c *Conversation) Get(organizationID, campaignID, conversationId string) error {
	doc, err := firestoredb.Client.Collection(fmt.Sprintf("organizations/%s/campaigns/%s/conversations", organizationID, campaignID)).Doc(conversationId).Get(context.Background())
	if err != nil {
		fmt.Println("Error getting item from Firestore:", err.Error())
		return err
	}
	err = doc.DataTo(c)
	if err != nil {
		fmt.Println("Error getting item from Firestore:", err.Error())
		return err
	}

	return nil
}

func (c *Conversation) GetByLead(leadId string) error {
	iter := firestoredb.Client.CollectionGroup("conversations").Query.Where("leadId", "==", leadId).Documents(context.Background())
	data, err := iter.Next()
	if err != nil {
		fmt.Println("Error getting item from Firestore:", err.Error())
		return err
	}
	err = data.DataTo(c)
	if err != nil {
		fmt.Println("Error getting item from Firestore:", err.Error())
		return err
	}

	return nil
}

func (c *Conversation) UpdateLastMID(mid string) (*firestore.WriteResult, error) {
	c.LastMID = mid

	path, err := c.GetPath()
	if err != nil {
		return nil, err
	}

	// Perform the update operation
	result, err := firestoredb.Client.Collection(*path).Doc(c.LeadID).Set(context.Background(), map[string]interface{}{
		"lastMid": mid,
	}, firestore.MergeAll)
	if err != nil {
		fmt.Println("Error updating item:", err)
		return nil, err
	}
	return result, nil
}

func (c *Conversation) UpdateProfileFetched() (*firestore.WriteResult, error) {
	c.IsProfileFetched = true

	path, err := c.GetPath()
	if err != nil {
		return nil, err
	}

	// Perform the update operation
	result, err := firestoredb.Client.Collection(*path).Doc(c.LeadID).Set(context.Background(), map[string]interface{}{
		"isProfileFetched": true,
	}, firestore.MergeAll)
	if err != nil {
		fmt.Println("Error updating item:", err)
		return nil, err
	}
	return result, nil
}

func GetConversations(organizationID string, campaignID string, sourceID *string, phase *int) ([]Conversation, error) {
	var conversations []Conversation

	query := firestoredb.Client.Collection(fmt.Sprintf("organizations/%s/campaigns/%s/conversations", organizationID, campaignID)).Query
	if sourceID != nil {
		query = query.Where("sourceId", "==", *sourceID)
	}
	if phase != nil {
		query = query.Where("phase", "==", *phase)
	}

	iter := query.Documents(context.Background())
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		conv := Conversation{}
		err = doc.DataTo(&conv)
		if err != nil {
			continue
		}
		conversations = append(conversations, conv)
	}

	return conversations, nil
}
