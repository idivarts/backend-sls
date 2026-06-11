package websocket

import (
	"context"
	"log"

	"github.com/aws/aws-lambda-go/events"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

func disconnectHandler(_ context.Context, event events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {
	connectionID := event.RequestContext.ConnectionID

	err := trendlymodels.DeleteWebsocketConnection(connectionID)
	if err != nil {
		log.Printf("Failed to disconnect: %v", err)
		return events.APIGatewayProxyResponse{StatusCode: 500, Body: "Failed to disconnect."}, nil
	}

	return events.APIGatewayProxyResponse{StatusCode: 200, Body: "Disconnected."}, nil
}
