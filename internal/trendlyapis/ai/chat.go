package ai

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

func handleMessageWS(req WSRequest) {
	ctx := context.Background()
	if req.ConversationID == "" {
		wsErrorTo(req.ConnectionID, "conversationId is required")
		return
	}

	conv, err := openrouter.GetConversation(ctx, req.ConversationID)
	if err != nil {
		wsErrorTo(req.ConnectionID, "conversation not found")
		return
	}
	if conv.UserID != req.UserID {
		wsErrorTo(req.ConnectionID, "forbidden")
		return
	}

	brand, err := loadBrand(conv.BrandID)
	if err != nil {
		wsErrorTo(req.ConnectionID, "brand not found")
		return
	}

	history, _ := openrouter.LoadHistory(ctx, conv.ID)

	systemPrompt := buildSystemPrompt(brand, conv.Module, conv.BrandID, conv.ContextID, req.FocusedText)

	msgs := make([]openrouter.Message, 0, len(history)+2)
	msgs = append(msgs, openrouter.Message{Role: "system", Content: systemPrompt})
	msgs = append(msgs, openrouter.ToOpenRouterMessages(history)...)
	msgs = append(msgs, openrouter.Message{Role: "user", Content: req.Content})

	if _, err := openrouter.AppendMessage(ctx, conv.ID, trendlymodels.AIMessage{
		Role:        "user",
		Content:     req.Content,
		FocusedText: req.FocusedText,
		Timestamp:   time.Now().UnixMilli(),
	}); err != nil {
		log.Printf("ai chat: persist user msg: %v", err)
	}

	if strings.TrimSpace(conv.Title) == "" || conv.Title == "New chat" {
		title := req.Content
		if len(title) > 60 {
			title = title[:60]
		}
		_ = openrouter.UpdateConversationTitle(ctx, conv.ID, title)
	}

	model := openrouter.ResolveModel(openrouter.TaskChat, req.Model)
	if model != conv.CurrentModel {
		_ = openrouter.UpdateConversationModel(ctx, conv.ID, model)
	}

	var assistant strings.Builder
	var finalUsage *openrouter.Usage

	streamErr := openrouter.ChatCompletionStream(ctx, openrouter.ChatRequest{
		Model:    model,
		Messages: msgs,
	}, openrouter.StreamCallbacks{
		OnDelta: func(delta string) {
			assistant.WriteString(delta)
			wsSend(req.ConnectionID, map[string]any{
				"type":           "token",
				"conversationId": conv.ID,
				"delta":          delta,
			})
		},
		OnDone: func(u *openrouter.Usage) {
			finalUsage = u
		},
		OnError: func(e error) {
			wsErrorTo(req.ConnectionID, e.Error())
		},
	})

	if streamErr != nil {
		log.Printf("ai chat stream: %v", streamErr)
		wsErrorTo(req.ConnectionID, streamErr.Error())
		return
	}

	tokens := 0
	if finalUsage != nil {
		tokens = finalUsage.TotalTokens
	}
	_, _ = openrouter.AppendMessage(ctx, conv.ID, trendlymodels.AIMessage{
		Role:       "assistant",
		Content:    assistant.String(),
		Model:      model,
		TokenCount: tokens,
		Timestamp:  time.Now().UnixMilli(),
	})

	wsSend(req.ConnectionID, map[string]any{
		"type":           "done",
		"conversationId": conv.ID,
		"usage":          finalUsage,
	})
}

type httpMessageReq struct {
	Content     string `json:"content" binding:"required"`
	FocusedText string `json:"focusedText"`
	Model       string `json:"model"`
}

func HTTPMessage(c *gin.Context) {
	conversationID := c.Param("conversationId")
	if conversationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "conversationId required"})
		return
	}
	managerID, _ := middlewares.GetUserId(c)

	var req httpMessageReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	conv, err := openrouter.GetConversation(ctx, conversationID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "conversation not found"})
		return
	}
	if conv.UserID != managerID {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	brand, err := loadBrand(conv.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "brand not found"})
		return
	}

	history, _ := openrouter.LoadHistory(ctx, conv.ID)
	systemPrompt := buildSystemPrompt(brand, conv.Module, conv.BrandID, conv.ContextID, req.FocusedText)

	msgs := []openrouter.Message{{Role: "system", Content: systemPrompt}}
	msgs = append(msgs, openrouter.ToOpenRouterMessages(history)...)
	msgs = append(msgs, openrouter.Message{Role: "user", Content: req.Content})

	model := openrouter.ResolveModel(openrouter.TaskChat, req.Model)

	resp, err := openrouter.ChatCompletion(ctx, openrouter.ChatRequest{
		Model:    model,
		Messages: msgs,
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	answer := ""
	if len(resp.Choices) > 0 {
		answer = resp.Choices[0].Message.Content
	}

	_, _ = openrouter.AppendMessage(ctx, conv.ID, trendlymodels.AIMessage{
		Role: "user", Content: req.Content, FocusedText: req.FocusedText,
		Timestamp: time.Now().UnixMilli(),
	})
	_, _ = openrouter.AppendMessage(ctx, conv.ID, trendlymodels.AIMessage{
		Role: "assistant", Content: answer, Model: model,
		Timestamp: time.Now().UnixMilli(),
	})
	_ = openrouter.UpdateConversationModel(ctx, conv.ID, model)

	c.JSON(http.StatusOK, gin.H{
		"conversationId": conv.ID,
		"content":        answer,
		"model":          model,
		"usage":          resp.Usage,
	})
}
