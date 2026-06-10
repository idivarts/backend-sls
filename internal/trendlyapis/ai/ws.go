package ai

import (
	"encoding/json"
	"log"

	wshandler "github.com/idivarts/backend-sls/pkg/ws_handler"
)

type WSRequest struct {
	ConnectionID string
	UserID       string

	Type string

	BrandID        string
	ConversationID string
	ClientMsgID    string
	Content        string
	FocusedText    string
	Model          string
	Module         string
	ContextID      string
	SelectedText   string
	Prompt         string
	Task           string
	Payload        map[string]any
}

func HandleWS(req WSRequest) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("ai HandleWS panic: %v", r)
			wsErrorTo(req.ConnectionID, "internal error")
		}
	}()

	switch req.Type {
	case "message":
		handleMessageWS(req)
	case "quick_edit":
		handleQuickEditWS(req)
	case "content_gen":
		handleContentGenWS(req)
	default:
		wsErrorTo(req.ConnectionID, "unknown type: "+req.Type)
	}
}

func wsSend(conn string, payload any) {
	b, err := json.Marshal(payload)
	if err != nil {
		log.Printf("ai wsSend marshal: %v", err)
		return
	}
	wshandler.SendToConnection(&conn, string(b))
}

func wsErrorTo(conn, message string) {
	wsSend(conn, map[string]any{"type": "error", "message": message})
}
