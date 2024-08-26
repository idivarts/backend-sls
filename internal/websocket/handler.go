package websocket

import (
	"context"
	"fmt"

	"github.com/aws/aws-lambda-go/events"
)

func Handler(ctx context.Context, event events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {
	switch event.RequestContext.RouteKey {
	case "$connect":
		// Handle new connection
		return connectHandler(ctx, event)
	case "$disconnect":
		// Handle disconnection
		return disconnectHandler(ctx, event)
	case "$default":
		// Handle any messages that do not match a specific route
		return messageHandler(ctx, event)
	default:
		return events.APIGatewayProxyResponse{StatusCode: 200, Body: fmt.Sprintf("Custom route: %s", event.RequestContext.RouteKey)}, nil
	}
}
