package models

import (
	"fmt"
	"log"
	"strconv"

	openaifc "github.com/TrendsHub/th-backend/internal/openai/fc"
	dynamodbhandler "github.com/TrendsHub/th-backend/pkg/dynamodb_handler"
	"github.com/TrendsHub/th-backend/pkg/messenger"
	"github.com/TrendsHub/th-backend/pkg/openai"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
)

type Conversation struct {
	OrganizationID     string            `json:"organizationId"`
	CampaignID         string            `json:"campaignId"`
	SourceID           string            `json:"sourceId"`
	ThreadID           string            `json:"threadId"`
	LeadID             string            `json:"leadId"`
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
	IGSID       string                 `json:"igsid" dynamodbav:"igsid"`
	UserProfile *messenger.UserProfile `json:"userProfile,omitempty" dynamodbav:"userProfile"`
	Information openaifc.ChangePhase   `json:"information" dynamodbav:"information"`
}

func (conversation *Conversation) CreateThread(includeLastMessage bool) error {
	pData := Source{}
	err := pData.Get(conversation.SourceID)
	if err != nil || pData.PageID == "" {
		return err
	}

	thread, err := openai.CreateThread()
	if err != nil {
		return err
	}
	threadId := thread.ID

	log.Println("Getting all conversations for this user")
	convIds, err := messenger.GetConversationsByUserId(conversation.IGSID, *pData.AccessToken)
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

	log.Println("Inserting the Conversation Model", conversation.IGSID, threadId)
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

func (c *Conversation) Insert() (*dynamodb.PutItemOutput, error) {
	data, err := dynamodbattribute.MarshalMap(*c)
	if err != nil {
		return nil, err
	}
	res, err := dynamodbhandler.Client.PutItem(&dynamodb.PutItemInput{
		TableName: aws.String(conversationTable),
		Item:      data,
	})
	return res, err
}

func (c *Conversation) Get(igsid string) error {
	result, err := dynamodbhandler.Client.GetItem(&dynamodb.GetItemInput{
		TableName: aws.String(conversationTable),
		Key: map[string]*dynamodb.AttributeValue{
			"igsid": {
				S: aws.String(igsid),
			},
		},
	})
	if err != nil {
		fmt.Println("Error getting item from DynamoDB:", err)
		return err
	}

	err = dynamodbattribute.UnmarshalMap(result.Item, c)
	if err != nil {
		fmt.Println("Error unmarshalling item:", err)
		return err
	}

	if c.IGSID == "" {
		return fmt.Errorf("error finding conversation %s", igsid)
	}

	return nil
}

func (c *Conversation) UpdateLastMID(mid string) (*dynamodb.UpdateItemOutput, error) {
	c.LastMID = mid
	// Specify the update expression and expression attribute values
	updateExpression := "SET #lastMid = :lastMid"
	expressionAttributeNames := map[string]*string{
		"#lastMid": aws.String("lastMid"),
	}
	expressionAttributeValues := map[string]*dynamodb.AttributeValue{
		":lastMid": {S: aws.String(c.LastMID)},
	}

	// Construct the update input
	input := &dynamodb.UpdateItemInput{
		TableName: aws.String(conversationTable),
		Key: map[string]*dynamodb.AttributeValue{
			"igsid": {
				S: aws.String(c.IGSID),
			},
		}, // Use the marshalled item as the key
		UpdateExpression:          aws.String(updateExpression),
		ExpressionAttributeNames:  expressionAttributeNames,
		ExpressionAttributeValues: expressionAttributeValues,
		ReturnValues:              aws.String("UPDATED_NEW"), // Specify the attributes to return after the update
	}

	// Perform the update operation
	result, err := dynamodbhandler.Client.UpdateItem(input)
	if err != nil {
		fmt.Println("Error updating item:", err)
		return nil, err
	}
	return result, nil
}

func (c *Conversation) UpdateProfileFetched() (*dynamodb.UpdateItemOutput, error) {
	c.IsProfileFetched = true
	// Specify the update expression and expression attribute values
	updateExpression := "SET #isProfileFetched = :isProfileFetched"
	expressionAttributeNames := map[string]*string{
		"#isProfileFetched": aws.String("isProfileFetched"),
	}
	expressionAttributeValues := map[string]*dynamodb.AttributeValue{
		":isProfileFetched": {BOOL: &c.IsProfileFetched},
	}

	// Construct the update input
	input := &dynamodb.UpdateItemInput{
		TableName: aws.String(conversationTable),
		Key: map[string]*dynamodb.AttributeValue{
			"igsid": {
				S: aws.String(c.IGSID),
			},
		}, // Use the marshalled item as the key
		UpdateExpression:          aws.String(updateExpression),
		ExpressionAttributeNames:  expressionAttributeNames,
		ExpressionAttributeValues: expressionAttributeValues,
		ReturnValues:              aws.String("UPDATED_NEW"), // Specify the attributes to return after the update
	}

	// Perform the update operation
	result, err := dynamodbhandler.Client.UpdateItem(input)
	if err != nil {
		fmt.Println("Error updating item:", err)
		return nil, err
	}
	return result, nil
}

// // Function to update the MessageQueue field in DynamoDB
// func (conversation Conversation) UpdateMessageQueue() error {
// 	// Update the MessageQueue field in the DynamoDB item
// 	input := &dynamodb.UpdateItemInput{
// 		TableName: aws.String(tableName),
// 		Key: map[string]*dynamodb.AttributeValue{
// 			"igsid": {
// 				S: aws.String(conversation.IGSID),
// 			},
// 		},
// 		ExpressionAttributeNames: map[string]*string{
// 			"#MQ": aws.String("messageQueue"),
// 		},
// 		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
// 			":mq": {
// 				SS: aws.StringSlice(conversation.MessageQueue),
// 			},
// 		},
// 		UpdateExpression: aws.String("SET #MQ = :mq"),
// 	}

// 	_, err := dynamodbhandler.Client.UpdateItem(input)
// 	if err != nil {
// 		return err
// 	}

// 	return nil
// }

// // Function to update the MessageQueue field in DynamoDB
// func (conversation Conversation) UpdateReminderQueue() error {
// 	// Update the MessageQueue field in the DynamoDB item
// 	input := &dynamodb.UpdateItemInput{
// 		TableName: aws.String(tableName),
// 		Key: map[string]*dynamodb.AttributeValue{
// 			"igsid": {
// 				S: aws.String(conversation.IGSID),
// 			},
// 		},
// 		ExpressionAttributeNames: map[string]*string{
// 			"#MQ": aws.String("reminderQueue"),
// 		},
// 		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
// 			":mq": {
// 				SS: aws.StringSlice(conversation.ReminderQueue),
// 			},
// 		},
// 		UpdateExpression: aws.String("SET #MQ = :mq"),
// 	}

// 	_, err := dynamodbhandler.Client.UpdateItem(input)
// 	if err != nil {
// 		return err
// 	}

// 	return nil
// }

func GetConversations(pageId *string, phase *int) ([]Conversation, error) {
	// Initialize AWS SDK and DynamoDB client

	// Initialize variables
	var conversations []Conversation

	// Create the input for the Scan operation
	input := &dynamodb.ScanInput{
		TableName: aws.String(conversationTable),
	}
	if pageId != nil {
		input = &dynamodb.ScanInput{
			TableName:        aws.String(conversationTable),
			FilterExpression: aws.String("pageId = :pageId"),
			ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
				":pageId": {
					S: pageId,
				},
			},
		}
	}
	if phase != nil {
		input = &dynamodb.ScanInput{
			TableName:        aws.String(conversationTable),
			FilterExpression: aws.String("currentPhase = :phase"),
			ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
				":phase": {
					N: aws.String(strconv.Itoa(*phase)),
				},
			},
		}
	}
	if pageId != nil && phase != nil {
		input = &dynamodb.ScanInput{
			TableName:        aws.String(conversationTable),
			FilterExpression: aws.String("pageId = :pageId AND currentPhase = :phase"),
			ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
				":pageId": {
					S: pageId,
				},
				":phase": {
					N: aws.String(strconv.Itoa(*phase)),
				},
			},
		}
	}

	// Perform the Scan operation
	result, err := dynamodbhandler.Client.Scan(input)
	if err != nil {
		return nil, err
	}

	// Parse the response into Conversation structs
	for _, item := range result.Items {
		conv := Conversation{}
		err = dynamodbattribute.UnmarshalMap(item, &conv)
		if err != nil {
			return nil, err
		}
		conversations = append(conversations, conv)
	}

	return conversations, nil
}
