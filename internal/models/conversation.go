package models

import (
	"fmt"

	dynamodbhandler "github.com/TrendsHub/th-backend/pkg/dynamodb_handler"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
)

const (
	tableName string = "conversationTable"
)

type Conversation struct {
	IGSID    string `json:"igsid" dynamodbav:"igsid"`
	ThreadID string `json:"threadId" dynamodbav:"threadId"`
	LastMID  string `json:"lastMid" dynamodbav:"lastMid"`
}

func (c *Conversation) Insert() (*dynamodb.PutItemOutput, error) {
	data, err := dynamodbattribute.MarshalMap(*c)
	if err != nil {
		return nil, err
	}
	res, err := dynamodbhandler.Client.PutItem(&dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item:      data,
	})
	return res, err
}

func (c *Conversation) Get(igsid string) error {
	result, err := dynamodbhandler.Client.GetItem(&dynamodb.GetItemInput{
		TableName: aws.String(tableName),
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
		TableName: aws.String(tableName),
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
