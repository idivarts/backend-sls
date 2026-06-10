package websocket

import (
	"context"
	"encoding/json"
	"log"

	"github.com/aws/aws-lambda-go/events"
	wshandler "github.com/idivarts/backend-sls/pkg/ws_handler"
)

type Envelope struct {
	Type string `json:"type,omitempty"`

	Action string `json:"action,omitempty"`
	Data   string `json:"data,omitempty"`

	BrandID        string         `json:"brandId,omitempty"`
	ConversationID string         `json:"conversationId,omitempty"`
	ClientMsgID    string         `json:"clientMsgId,omitempty"`
	Content        string         `json:"content,omitempty"`
	FocusedText    string         `json:"focusedText,omitempty"`
	Model          string         `json:"model,omitempty"`
	Module         string         `json:"module,omitempty"`
	ContextID      string         `json:"contextId,omitempty"`
	SelectedText   string         `json:"selectedText,omitempty"`
	Prompt         string         `json:"prompt,omitempty"`
	Task           string         `json:"task,omitempty"`
	Payload        map[string]any `json:"payload,omitempty"`
}

func messageHandler(_ context.Context, event events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {
	connectionID := event.RequestContext.ConnectionID

	var env Envelope
	if err := json.Unmarshal([]byte(event.Body), &env); err != nil {
		log.Printf("Failed to unmarshal message: %v", err)
		return events.APIGatewayProxyResponse{StatusCode: 400, Body: "Invalid message format."}, nil
	}

	switch env.Type {
	case "message", "quick_edit", "content_gen", "push_to_calendar":
		userID, ok := lookupUserID(connectionID)
		if !ok {
			sendError(connectionID, "unauthenticated")
			return events.APIGatewayProxyResponse{StatusCode: 401, Body: "Unauthenticated"}, nil
		}
		dispatchAI(connectionID, userID, env)
		return events.APIGatewayProxyResponse{StatusCode: 200, Body: "OK"}, nil
	}

	if env.Action != "" || env.Data != "" {
		if err := wshandler.Broadcast(env.Data); err != nil {
			log.Printf("Broadcast failed: %v", err)
			return events.APIGatewayProxyResponse{StatusCode: 400, Body: "Broadcast failed."}, nil
		}
		return events.APIGatewayProxyResponse{StatusCode: 200, Body: "Message sent."}, nil
	}

	return events.APIGatewayProxyResponse{StatusCode: 200, Body: "Ignored."}, nil
}

func lookupUserID(connectionID string) (string, bool) {
	doc, err := firestoreClient.Collection("websockets").Doc(connectionID).Get(context.Background())
	if err != nil {
		return "", false
	}
	data := doc.Data()
	uid, ok := data["userId"].(string)
	if !ok || uid == "" {
		return "", false
	}
	return uid, true
}

func sendError(connectionID string, message string) {
	payload, _ := json.Marshal(map[string]any{"type": "error", "message": message})
	wshandler.SendToConnection(&connectionID, string(payload))
}
