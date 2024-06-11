package websocket

import (
	"context"
	"log"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
)

func ConnectHandler(ctx context.Context, event events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {
	connectionID := event.RequestContext.ConnectionID

	item := map[string]*dynamodb.AttributeValue{
		"connectionId": {
			S: aws.String(connectionID),
		},
	}

	_, err := dynamoClient.PutItem(&dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item:      item,
	})
	if err != nil {
		log.Printf("Failed to connect: %v", err)
		return events.APIGatewayProxyResponse{StatusCode: 500, Body: "Failed to connect."}, nil
	}

	return events.APIGatewayProxyResponse{StatusCode: 200, Body: "Connected."}, nil
}
