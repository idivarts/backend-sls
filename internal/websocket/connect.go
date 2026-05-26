package websocket

import (
	"context"
	"log"

	"github.com/aws/aws-lambda-go/events"
	"github.com/idivarts/backend-sls/pkg/firebase/fauth"
)

func connectHandler(_ context.Context, event events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {
	connectionID := event.RequestContext.ConnectionID

	doc := map[string]interface{}{
		"connected":    true,
		"connectionId": connectionID,
		"connectedAt":  event.RequestContext.RequestTimeEpoch,
	}

	if token := event.QueryStringParameters["token"]; token != "" {
		decoded, err := fauth.Client.VerifyIDToken(context.Background(), token)
		if err == nil {
			doc["userId"] = decoded.UID
		} else {
			log.Printf("ws connect: invalid token: %v", err)
		}
	}

	_, err := firestoreClient.Collection("websockets").Doc(connectionID).Set(context.Background(), doc)
	if err != nil {
		log.Printf("Failed to connect: %v", err)
		return events.APIGatewayProxyResponse{StatusCode: 500, Body: "Failed to connect."}, nil
	}

	return events.APIGatewayProxyResponse{StatusCode: 200, Body: "Connected."}, nil
}
