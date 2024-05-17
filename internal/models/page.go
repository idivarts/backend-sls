package models

import (
	"fmt"

	dynamodbhandler "github.com/TrendsHub/th-backend/pkg/dynamodb_handler"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
)

//	type InstagramObject struct {
//		ID       string `json:"id" dynamodbav:"id"`
//		Name     string `json:"name" dynamodbav:"name"`
//		UserName string `json:"userName" dynamodbav:"userName"`
//		Bio      string `json:"bio" dynamodbav:"bio"`
//	}
type Page struct {
	PageID      string `json:"pageId" dynamodbav:"pageId"`
	ConnectedID string `json:"connectedId" dynamodbav:"connectedId"`
	UserID      string `json:"userId" dynamodbav:"userId"`
	OwnerName   string `json:"ownerName" dynamodbav:"ownerName"`
	Name        string `json:"name" dynamodbav:"name"`
	UserName    string `json:"userName" dynamodbav:"userName"`
	Bio         string `json:"bio" dynamodbav:"bio"`
	IsInstagram bool   `json:"isInstagram" dynamodbav:"isInstagram"`
	AccessToken string `json:"accessToken" dynamodbav:"accessToken"`
	AssistantID string `json:"assistantId" dynamodbav:"assistantId"`
	Status      int    `json:"status" dynamodbav:"status"`
	// Instagram   *InstagramObject `json:"instagram,omitempty"`
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
			"pageId": {
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
	input := &dynamodb.ScanInput{
		TableName:        aws.String(pageTable),
		FilterExpression: aws.String("#userId = :v_userId"),
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
	result, err := dynamodbhandler.Client.Scan(input)
	if err != nil {
		fmt.Println("Error scanning DynamoDB table:", err)
		return nil, err
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

func FetchAllPages() ([]Page, error) {

	// Input parameters for Scan operation
	input := &dynamodb.ScanInput{
		TableName: aws.String(pageTable),
	}

	// Perform a Scan operation to fetch all items
	result, err := dynamodbhandler.Client.Scan(input)
	if err != nil {
		return nil, err
	}

	// Unmarshal the DynamoDB items into a slice of Page structs
	pages := make([]Page, 0)
	for _, item := range result.Items {
		page := Page{}
		err = dynamodbattribute.UnmarshalMap(item, &page)
		if err != nil {
			return nil, err
		}
		pages = append(pages, page)
	}

	return pages, nil
}
