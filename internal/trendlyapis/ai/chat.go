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
	readtools "github.com/idivarts/backend-sls/internal/trendlyapis/ai/tools"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// maxToolSteps caps the agentic loop so a misbehaving model can't spin forever
// calling server tools without ever producing a user-facing turn.
const maxToolSteps = 8

// handleStopWS records a cancel request for the conversation's in-flight turn.
// The turn runs synchronously in a different WS (Lambda) invocation, so we can't
// touch its context directly — we drop a Firestore marker the streaming loop
// polls and honors. The client returns control to the user immediately on its
// side; this just makes the server actually stop generating (and stop billing
// tokens) instead of running to completion in the background.
func handleStopWS(req WSRequest) {
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
	if err := openrouter.RequestCancel(ctx, conv.ID); err != nil {
		log.Printf("ai chat: request cancel: %v", err)
	}
	// Ack so the client knows the stop was received. The running turn emits its
	// own terminal `done` (with `stopped: true`) once it actually breaks; if the
	// turn had already finished, this ack is the only reply — harmless.
	wsSend(req.ConnectionID, map[string]any{
		"type":           "stop_ack",
		"conversationId": conv.ID,
	})
}

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

	// Keep the client's streaming watchdog alive for the whole turn. The turn
	// runs synchronously here and can go silent for a while (a slow server tool,
	// a slow model round-trip between tool steps) with no token deltas; the
	// heartbeat means the watchdog only fires when the turn is genuinely dead.
	stopHeartbeat := startHeartbeat(req.ConnectionID, conv.ID)
	defer stopHeartbeat()

	history, _ := openrouter.LoadHistory(ctx, conv.ID)

	systemPrompt := buildSystemPrompt(brand, conv.Module, conv.BrandID, conv.ContextID)
	// Content module: prefer the live (possibly unsaved) editor state the client
	// sends with the message over the last-saved Firestore doc, so the AI reasons
	// about exactly what's on screen right now (same pattern as content generation).
	if conv.Module == moduleContent && len(req.Payload) > 0 {
		var live liveContentPayload
		if err := decodePayload(req.Payload, &live); err == nil {
			if brief := briefFromLiveContent(live); brief != "" {
				systemPrompt = systemPromptWithLiveContent(brand, conv.BrandID, conv.ContextID, brief)
			}
		}
	}
	// Tell the content chat there's a design and how to work with it — a small
	// REFERENCE only (revision id + shape), not the HTML itself. The model fetches
	// the full HTML on demand via get_design_html so a large design never truncates
	// the prompt.
	if conv.Module == moduleContent && conv.ContextID != "" {
		if design := currentDesignBrief(conv.BrandID, conv.ContextID); design != "" {
			systemPrompt = systemPrompt + "\n\n" + design
		}
	}

	msgs := make([]openrouter.Message, 0, len(history)+2)
	msgs = append(msgs, openrouter.Message{Role: "system", Content: systemPrompt})
	msgs = append(msgs, openrouter.ToOpenRouterMessages(history)...)
	// Attach the focus to THIS (latest) user turn — not the system prompt. Models
	// weight the newest message most, so a "[Focused on: …]" prefix on the current
	// turn is respected, whereas the same info in the system prompt is often
	// treated as stale and the model re-asks "which one?".
	userContent := req.Content
	if note := (trendlymodels.AIMessage{Focus: req.Focus, FocusedText: req.FocusedText}).FocusNote(); note != "" {
		userContent = note + "\n" + req.Content
	}
	// When the user attached images, send the turn as multimodal vision input.
	if len(req.Images) > 0 {
		msgs = append(msgs, openrouter.UserMessageWithImages(userContent, req.Images))
	} else {
		msgs = append(msgs, openrouter.Message{Role: "user", Content: userContent})
	}

	if _, err := openrouter.AppendMessage(ctx, conv.ID, trendlymodels.AIMessage{
		Role:        "user",
		UserID:      conv.UserID,
		BrandID:     conv.BrandID,
		ClientMsgID: req.ClientMsgID,
		Content:     req.Content,
		Images:      req.Images,
		FocusedText: req.FocusedText,
		Focus:       req.Focus,
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

	// Image-bearing turns must run on a vision-capable model — route them through
	// the multimodal task (its allowed models are all vision-capable).
	chatTask := openrouter.TaskChat
	if len(req.Images) > 0 {
		chatTask = openrouter.TaskMultimodal
	}
	model, locked := pickModel(ctx, conv.BrandID, chatTask, req.Model)
	if locked {
		wsSend(req.ConnectionID, map[string]any{
			"type":           "upgrade_required",
			"conversationId": conv.ID,
			"task":           string(chatTask),
		})
		return
	}
	orgID, _ := orgIDForBrand(conv.BrandID)
	if aiTokensExhausted(orgID) {
		wsSend(req.ConnectionID, map[string]any{
			"type":           "upgrade_required",
			"reason":         "tokens_exhausted",
			"conversationId": conv.ID,
			"task":           string(chatTask),
		})
		return
	}
	if model != conv.CurrentModel {
		_ = openrouter.UpdateConversationModel(ctx, conv.ID, model)
	}

	tools := toolsForModule(conv.Module)

	// ── Cooperative cancellation ──────────────────────────────────────────
	// The user can interrupt this turn from the composer (Stop button). The stop
	// arrives on a separate WS invocation as a `cancelRequestedAt` marker on the
	// conversation; we poll it (throttled to ~1/s so it costs ~one Firestore read
	// per second of streaming) and, when it's newer than this turn's start, cancel
	// the model stream and break — committing whatever streamed so far.
	turnStart := time.Now().UnixMilli()
	streamCtx, cancelStream := context.WithCancel(ctx)
	defer cancelStream()
	cancelled := false
	lastCancelCheck := time.Now()
	checkCancel := func() {
		if cancelled {
			return
		}
		if time.Since(lastCancelCheck) < time.Second {
			return
		}
		lastCancelCheck = time.Now()
		if at, err := openrouter.GetCancelRequestedAt(ctx, conv.ID); err == nil && at >= turnStart {
			cancelled = true
			cancelStream()
		}
	}

	var fullText strings.Builder // cumulative across steps — matches what the client accumulates
	var finalUsage *openrouter.Usage
	var pendingControl *trendlymodels.AIControl
	// Images produced by generate_image tool calls this turn — attached to the
	// committed assistant message so the chat bubble renders them from Firestore.
	var genImages []string
	// completed is set by a terminal server tool (complete_onboarding /
	// generate_strategy_doc); the matching WS signal is chosen by module below.
	completed := false

	for step := 0; step < maxToolSteps; step++ {
		// Honor a cancel requested between steps (e.g. during a token-silent
		// server-tool phase, where OnDelta isn't firing).
		checkCancel()
		if cancelled {
			break
		}

		var stepText strings.Builder
		var toolCalls []openrouter.ToolCall
		var streamErr error

		err := openrouter.ChatCompletionStream(streamCtx, openrouter.ChatRequest{
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
				checkCancel()
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
		// A cancel tears down streamCtx, which surfaces as a context error here —
		// that's an intended stop, not a failure. Break and commit the partial.
		if cancelled {
			break
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
				// Tell the user what's happening during this (token-silent) tool
				// call — also re-arms the client watchdog.
				wsStatus(req.ConnectionID, conv.ID, toolStatusLabel(sc.Function.Name))
				var result string
				var complete bool
				var derr error
				if sc.Function.Name == toolGenerateImage {
					var urls []string
					result, urls, derr = runChatImageTool(ctx, conv.BrandID, orgID, req.ConnectionID, conv.ID, req.Model, sc.Function.Arguments)
					genImages = append(genImages, urls...)
				} else {
					result, complete, derr = dispatchServerTool(ctx, conv.BrandID, conv.UserID, conv.ContextID, sc.Function.Name, sc.Function.Arguments)
				}
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
	meterAIUsage(orgID, finalUsage)
	// On a cancel we still persist whatever streamed so it isn't lost — unless
	// nothing came through yet (interrupted during "Thinking…"), in which case
	// there's no assistant bubble to write.
	var assistantMsgID string
	if !(cancelled && strings.TrimSpace(fullText.String()) == "" && len(genImages) == 0 && pendingControl == nil) {
		assistantMsgID, _ = openrouter.AppendMessage(ctx, conv.ID, trendlymodels.AIMessage{
			Role:       "assistant",
			UserID:     conv.UserID,
			BrandID:    conv.BrandID,
			Content:    fullText.String(),
			Images:     genImages,
			Model:      model,
			TokenCount: tokens,
			Control:    pendingControl,
			Timestamp:  time.Now().UnixMilli(),
		})
	}

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
		"images":         genImages,
		"usage":          finalUsage,
		"stopped":        cancelled,
	})
}

// toolsForModule returns the tools available to a conversation. Answer-control
// tools (ask_options / ask_input) are available everywhere; onboarding adds the
// brand-building server tools.
func toolsForModule(module string) []openrouter.Tool {
	tools := clientTools()
	// The brand-memory tool is available in every module so the AI can capture
	// durable brand facts from any conversation.
	tools = append(tools, memoryServerTools()...)
	if module == moduleOnboarding {
		tools = append(tools, onboardingServerTools()...)
	}
	if module == moduleStrategy {
		tools = append(tools, strategyServerTools()...)
	}
	if module == moduleCalendar {
		tools = append(tools, calendarServerTools()...)
	}
	// Image generation is offered on the content-creation chat surfaces.
	if moduleHasImageGen(module) {
		tools = append(tools, imageGenServerTools()...)
	}
	// The AI-Studio HTML design editor + audio generation live on the content module.
	if moduleHasStudio(module) {
		tools = append(tools, designServerTools()...)
		tools = append(tools, audioServerTools()...)
	}
	// On-demand read-only fetch tools (content, analytics, inbox, strategy,
	// account, assets, billing) let the AI ground its output in the brand's real
	// data. Available on the planner surfaces — not during onboarding (no data
	// yet) nor on the lean media image-gen surface.
	if module != moduleOnboarding && module != moduleMedia {
		tools = append(tools, readtools.AllTools()...)
	}
	return tools
}

type httpMessageReq struct {
	Content     string                  `json:"content" binding:"required"`
	FocusedText string                  `json:"focusedText"`
	Focus       []trendlymodels.AIFocus `json:"focus"`
	Model       string                  `json:"model"`
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
	systemPrompt := buildSystemPrompt(brand, conv.Module, conv.BrandID, conv.ContextID)

	msgs := []openrouter.Message{{Role: "system", Content: systemPrompt}}
	msgs = append(msgs, openrouter.ToOpenRouterMessages(history)...)
	// Attach focus to the latest user turn (not the system prompt) — see the WS
	// handler for why.
	httpUserContent := req.Content
	if note := (trendlymodels.AIMessage{Focus: req.Focus, FocusedText: req.FocusedText}).FocusNote(); note != "" {
		httpUserContent = note + "\n" + req.Content
	}
	msgs = append(msgs, openrouter.Message{Role: "user", Content: httpUserContent})

	model, locked := pickModel(ctx, conv.BrandID, openrouter.TaskChat, req.Model)
	if locked {
		c.JSON(http.StatusPaymentRequired, gin.H{"error": "upgrade_required", "task": openrouter.TaskChat})
		return
	}

	orgID, _ := orgIDForBrand(conv.BrandID)
	if aiTokensExhausted(orgID) {
		c.JSON(http.StatusPaymentRequired, gin.H{"error": "upgrade_required", "reason": "tokens_exhausted", "task": openrouter.TaskChat})
		return
	}

	resp, err := openrouter.ChatCompletion(ctx, openrouter.ChatRequest{
		Model:    model,
		Messages: msgs,
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	meterAIUsage(orgID, resp.Usage)

	answer := ""
	if len(resp.Choices) > 0 {
		answer = resp.Choices[0].Message.Content
	}

	_, _ = openrouter.AppendMessage(ctx, conv.ID, trendlymodels.AIMessage{
		Role: "user", Content: req.Content, FocusedText: req.FocusedText, Focus: req.Focus,
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
