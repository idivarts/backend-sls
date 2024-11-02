package websocket

import (
	"context"
	"encoding/json"
	"log"

	"github.com/aws/aws-lambda-go/events"
	wshandler "github.com/idivarts/backend-sls/pkg/ws_handler"
)

type Message struct {
	Action string `json:"action"`
	Data   string `json:"data"`
}

func messageHandler(_ context.Context, event events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {
	var msg Message
	err := json.Unmarshal([]byte(event.Body), &msg)
	if err != nil {
		log.Printf("Failed to unmarshal message: %v", err)
		return events.APIGatewayProxyResponse{StatusCode: 400, Body: "Invalid message format."}, nil
	}

	err = wshandler.Broadcast(msg.Data)
	if err != nil {
		log.Printf("Failed to unmarshal message: %v", err)
		return events.APIGatewayProxyResponse{StatusCode: 400, Body: "Invalid message format."}, nil
	}

	return events.APIGatewayProxyResponse{StatusCode: 200, Body: "Message sent."}, nil
}
