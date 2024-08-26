package websocket

import (
	"context"
	"log"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
)

func disconnectHandler(ctx context.Context, event events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {
	connectionID := event.RequestContext.ConnectionID

	_, err := dynamoClient.DeleteItem(&dynamodb.DeleteItemInput{
		TableName: aws.String(tableName),
		Key: map[string]*dynamodb.AttributeValue{
			"connectionId": {
				S: aws.String(connectionID),
			},
		},
	})
	if err != nil {
		log.Printf("Failed to disconnect: %v", err)
		return events.APIGatewayProxyResponse{StatusCode: 500, Body: "Failed to disconnect."}, nil
	}

	return events.APIGatewayProxyResponse{StatusCode: 200, Body: "Disconnected."}, nil
}
