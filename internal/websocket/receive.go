package websocket

import (
	"context"
	"encoding/json"
	"log"

	wshandler "github.com/TrendsHub/th-backend/pkg/ws_handler"
	"github.com/aws/aws-lambda-go/events"
)

type Message struct {
	Action string `json:"action"`
	Data   string `json:"data"`
}

func MessageHandler(ctx context.Context, event events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {
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
