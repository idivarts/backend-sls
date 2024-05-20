package models

import (
	"fmt"

	openaifc "github.com/TrendsHub/th-backend/internal/openai/fc"
	dynamodbhandler "github.com/TrendsHub/th-backend/pkg/dynamodb_handler"
	"github.com/TrendsHub/th-backend/pkg/messenger"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
)

type Conversation struct {
	IGSID              string                 `json:"igsid" dynamodbav:"igsid"`
	PageID             string                 `json:"pageId" dynamodbav:"pageId"`
	ThreadID           string                 `json:"threadId" dynamodbav:"threadId"`
	LastMID            string                 `json:"lastMid" dynamodbav:"lastMid"`
	LastBotMessageTime int64                  `json:"lastBotMessageTime" dynamodbav:"lastBotMessageTime"`
	IsProfileFetched   bool                   `json:"isProfileFetched" dynamodbav:"isProfileFetched"`
	UserProfile        *messenger.UserProfile `json:"userProfile,omitempty" dynamodbav:"userProfile"`
	Phases             []int                  `json:"phases" dynamodbav:"phases"`
	CurrentPhase       int                    `json:"currentPhase" dynamodbav:"currentPhase"`
	Information        openaifc.ChangePhase   `json:"information" dynamodbav:"information"`
	MessageQueue       *string                `json:"messageQueue" dynamodbav:"messageQueue"`
	ReminderQueue      *string                `json:"reminderQueue" dynamodbav:"reminderQueue"`
	ReminderCount      int                    `json:"reminderCount" dynamodbav:"reminderCount"`
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
