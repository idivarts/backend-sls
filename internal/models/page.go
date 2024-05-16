package models

import (
	"fmt"
	"os"

	dynamodbhandler "github.com/TrendsHub/th-backend/pkg/dynamodb_handler"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
)

type Page struct {
	PageID      string `json:"pageId" dynamodbav:"pageId"`
	UserID      string `json:"userId" dynamodbav:"userId"`
	AccessToken string `json:"accessToken" dynamodbav:"accessToken"`
	Status      int    `json:"status" dynamodbav:"status"`
}

func (c *Page) Insert() (*dynamodb.PutItemOutput, error) {
	data, err := dynamodbattribute.MarshalMap(*c)
	if err != nil {
		return nil, err
	}
	res, err := dynamodbhandler.Client.PutItem(&dynamodb.PutItemInput{
		TableName: aws.String(pageTable),
		Item:      data,
	})
	return res, err
}

func (c *Page) Get(pageId string) error {
	result, err := dynamodbhandler.Client.GetItem(&dynamodb.GetItemInput{
		TableName: aws.String(pageTable),
		Key: map[string]*dynamodb.AttributeValue{
			"pageeId": {
				S: aws.String(pageId),
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

func GetPagesByUserId(userId string) ([]Page, error) {
	// Create the input for the query operation
	input := &dynamodb.QueryInput{
		TableName:              aws.String(pageTable),
		KeyConditionExpression: aws.String("#userId = :v_userId"),
		ExpressionAttributeNames: map[string]*string{
			"#userId": aws.String("userId"),
		},
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":v_userId": {
				S: aws.String(userId),
			},
		},
	}

	// Perform the query operation
	result, err := dynamodbhandler.Client.Query(input)
	if err != nil {
		fmt.Println("Error querying DynamoDB table:", err)
		os.Exit(1)
	}

	pages := []Page{}
	err = dynamodbattribute.UnmarshalListOfMaps(result.Items, &pages)
	if err != nil {
		fmt.Println("Error unmarshalling item:", err)
		return nil, err
	}

	// // Print the results
	// fmt.Println("Query results:")
	// for _, item := range result.Items {
	// 	fmt.Println(item)
	// }

	return pages, nil
}
