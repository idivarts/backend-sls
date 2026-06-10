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

// maxToolSteps caps the agentic loop so a misbehaving model can't spin forever
// calling server tools without ever producing a user-facing turn.
const maxToolSteps = 8

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
		UserID:      conv.UserID,
		BrandID:     conv.BrandID,
		ClientMsgID: req.ClientMsgID,
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

	tools := toolsForModule(conv.Module)

	var fullText strings.Builder // cumulative across steps — matches what the client accumulates
	var finalUsage *openrouter.Usage
	var pendingControl *trendlymodels.AIControl
	// completed is set by a terminal server tool (complete_onboarding /
	// generate_strategy_doc); the matching WS signal is chosen by module below.
	completed := false

	for step := 0; step < maxToolSteps; step++ {
		var stepText strings.Builder
		var toolCalls []openrouter.ToolCall
		var streamErr error

		err := openrouter.ChatCompletionStream(ctx, openrouter.ChatRequest{
			Model:    model,
			Messages: msgs,
			Tools:    tools,
		}, openrouter.StreamCallbacks{
			OnDelta: func(delta string) {
				stepText.WriteString(delta)
				fullText.WriteString(delta)
				wsSend(req.ConnectionID, map[string]any{
					"type":           "token",
					"conversationId": conv.ID,
					"delta":          delta,
				})
			},
			OnToolCall: func(call openrouter.ToolCall) {
				toolCalls = append(toolCalls, call)
			},
			OnDone:  func(u *openrouter.Usage) { finalUsage = u },
			OnError: func(e error) { streamErr = e },
		})
		if err != nil {
			streamErr = err
		}
		if streamErr != nil {
			log.Printf("ai chat stream: %v", streamErr)
			wsErrorTo(req.ConnectionID, streamErr.Error())
			return
		}

		// Partition tool calls: the first client tool is terminal; server tools
		// execute and (when alone) loop back into the model.
		var clientCall *openrouter.ToolCall
		var serverCalls []openrouter.ToolCall
		for i := range toolCalls {
			if isClientTool(toolCalls[i].Function.Name) {
				if clientCall == nil {
					c := toolCalls[i]
					clientCall = &c
				}
			} else {
				serverCalls = append(serverCalls, toolCalls[i])
			}
		}

		// Execute any server tools (e.g. set_brand_fields, complete_onboarding).
		if len(serverCalls) > 0 {
			// Echo back only the server calls we will answer, stripped of the
			// streaming-only index, so the assistant/tool messages stay balanced
			// even if a terminal client tool also appeared this step.
			echo := make([]openrouter.ToolCall, 0, len(serverCalls))
			for _, sc := range serverCalls {
				echo = append(echo, openrouter.ToolCall{
					ID:       sc.ID,
					Type:     sc.Type,
					Function: sc.Function,
				})
			}
			msgs = append(msgs, openrouter.Message{
				Role:      "assistant",
				Content:   stepText.String(),
				ToolCalls: echo,
			})
			for _, sc := range serverCalls {
				result, complete, derr := dispatchServerTool(ctx, conv.BrandID, conv.UserID, conv.ContextID, sc.Function.Name, sc.Function.Arguments)
				if derr != nil {
					log.Printf("ai server tool %s: %v", sc.Function.Name, derr)
				}
				if complete {
					completed = true
				}
				msgs = append(msgs, openrouter.Message{
					Role:       "tool",
					ToolCallID: sc.ID,
					Name:       sc.Function.Name,
					Content:    result,
				})
			}
		}

		// A client tool ends the turn — build the control and stop.
		if clientCall != nil {
			control, question, ok := buildControl(*clientCall)
			if ok {
				if stepText.Len() == 0 && question != "" {
					// The model emitted no prose, only the tool call. Surface the
					// question text so the user sees what's being asked.
					fullText.WriteString(question)
					wsSend(req.ConnectionID, map[string]any{
						"type":           "token",
						"conversationId": conv.ID,
						"delta":          question,
					})
				}
				pendingControl = control
			}
			break
		}

		// Only server tools this step → loop so the model can use their results.
		if len(serverCalls) > 0 {
			continue
		}

		// Plain text answer, no tools → done.
		break
	}

	tokens := 0
	if finalUsage != nil {
		tokens = finalUsage.TotalTokens
	}
	assistantMsgID, _ := openrouter.AppendMessage(ctx, conv.ID, trendlymodels.AIMessage{
		Role:       "assistant",
		UserID:     conv.UserID,
		BrandID:    conv.BrandID,
		Content:    fullText.String(),
		Model:      model,
		TokenCount: tokens,
		Control:    pendingControl,
		Timestamp:  time.Now().UnixMilli(),
	})

	if pendingControl != nil {
		wsSend(req.ConnectionID, map[string]any{
			"type":           "control",
			"conversationId": conv.ID,
			"control":        pendingControl,
		})
	}
	if completed {
		signal := "onboarding_complete"
		if conv.Module == moduleStrategy {
			signal = "strategy_ready"
		}
		wsSend(req.ConnectionID, map[string]any{
			"type":           signal,
			"conversationId": conv.ID,
		})
	}

	// The client reconciles its optimistic bubbles against Firestore using these
	// ids: messageId = the committed assistant doc, clientMsgId = the user's
	// optimistic bubble it can now drop in favor of the synced doc.
	wsSend(req.ConnectionID, map[string]any{
		"type":           "done",
		"conversationId": conv.ID,
		"messageId":      assistantMsgID,
		"clientMsgId":    req.ClientMsgID,
		"usage":          finalUsage,
	})
}

// toolsForModule returns the tools available to a conversation. Answer-control
// tools (ask_options / ask_input) are available everywhere; onboarding adds the
// brand-building server tools.
func toolsForModule(module string) []openrouter.Tool {
	tools := clientTools()
	if module == moduleOnboarding {
		tools = append(tools, onboardingServerTools()...)
	}
	if module == moduleStrategy {
		tools = append(tools, strategyServerTools()...)
	}
	if module == moduleCalendar {
		tools = append(tools, calendarServerTools()...)
	}
	return tools
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
