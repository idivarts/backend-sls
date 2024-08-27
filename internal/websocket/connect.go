package websocket

import (
	"context"
	"log"

	"github.com/aws/aws-lambda-go/events"
)

func connectHandler(_ context.Context, event events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {
	connectionID := event.RequestContext.ConnectionID

	// item := map[string]*dynamodb.AttributeValue{
	// 	"connectionId": {
	// 		S: aws.String(connectionID),
	// 	},
	// }

	_, err := firestoreClient.Collection("websockets").Doc(connectionID).Set(context.Background(), map[string]interface{}{
		"connected":    true,
		"connectionId": connectionID,
	})

	if err != nil {
		log.Printf("Failed to connect: %v", err)
		return events.APIGatewayProxyResponse{StatusCode: 500, Body: "Failed to connect."}, nil
	}

	return events.APIGatewayProxyResponse{StatusCode: 200, Body: "Connected."}, nil
}
