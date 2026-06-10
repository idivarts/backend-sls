package websocket

import (
	"github.com/idivarts/backend-sls/internal/trendlyapis/ai"
)

func dispatchAI(connectionID, userID string, env Envelope) {
	req := ai.WSRequest{
		ConnectionID:   connectionID,
		UserID:         userID,
		Type:           env.Type,
		BrandID:        env.BrandID,
		ConversationID: env.ConversationID,
		ClientMsgID:    env.ClientMsgID,
		Content:        env.Content,
		FocusedText:    env.FocusedText,
		Model:          env.Model,
		Module:         env.Module,
		ContextID:      env.ContextID,
		SelectedText:   env.SelectedText,
		Prompt:         env.Prompt,
		Task:           env.Task,
		Payload:        env.Payload,
	}
	ai.HandleWS(req)
}
