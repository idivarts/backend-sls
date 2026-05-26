package ai

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

func quickEditUserPrompt(selectedText, prompt string) string {
	return fmt.Sprintf(
		"Selected text:\n\"\"\"\n%s\n\"\"\"\n\nInstruction: %s\n\nReturn only the rewritten text, no commentary, no preamble.",
		selectedText, prompt,
	)
}

func handleQuickEditWS(req WSRequest) {
	if req.SelectedText == "" || req.Prompt == "" {
		wsErrorTo(req.ConnectionID, "selectedText and prompt are required")
		return
	}
	if req.BrandID == "" {
		wsErrorTo(req.ConnectionID, "brandId is required")
		return
	}
	if !verifyBrandAccess(req.BrandID, req.UserID) {
		wsErrorTo(req.ConnectionID, "forbidden")
		return
	}

	brand, err := loadBrand(req.BrandID)
	if err != nil {
		wsErrorTo(req.ConnectionID, "brand not found")
		return
	}

	systemPrompt := buildSystemPrompt(brand, req.Module, req.BrandID, req.ContextID, "")
	model := openrouter.ResolveModel(openrouter.TaskQuickEdit, req.Model)

	msgs := []openrouter.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: quickEditUserPrompt(req.SelectedText, req.Prompt)},
	}

	streamErr := openrouter.ChatCompletionStream(context.Background(), openrouter.ChatRequest{
		Model:    model,
		Messages: msgs,
	}, openrouter.StreamCallbacks{
		OnDelta: func(delta string) {
			wsSend(req.ConnectionID, map[string]any{
				"type":  "token",
				"delta": delta,
			})
		},
		OnDone: func(u *openrouter.Usage) {
			wsSend(req.ConnectionID, map[string]any{
				"type":  "done",
				"usage": u,
			})
		},
		OnError: func(e error) {
			wsErrorTo(req.ConnectionID, e.Error())
		},
	})

	if streamErr != nil {
		log.Printf("ai quickedit stream: %v", streamErr)
	}
}

type httpQuickEditReq struct {
	SelectedText string `json:"selectedText" binding:"required"`
	Prompt       string `json:"prompt" binding:"required"`
	Model        string `json:"model"`
	Module       string `json:"module"`
	ContextID    string `json:"contextId"`
	BrandID      string `json:"brandId" binding:"required"`
}

func HTTPQuickEdit(c *gin.Context) {
	managerID, _ := middlewares.GetUserId(c)
	var req httpQuickEditReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !verifyBrandAccess(req.BrandID, managerID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	brand, err := loadBrand(req.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "brand not found"})
		return
	}

	systemPrompt := buildSystemPrompt(brand, req.Module, req.BrandID, req.ContextID, "")
	model := openrouter.ResolveModel(openrouter.TaskQuickEdit, req.Model)

	resp, err := openrouter.ChatCompletion(c.Request.Context(), openrouter.ChatRequest{
		Model: model,
		Messages: []openrouter.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: quickEditUserPrompt(req.SelectedText, req.Prompt)},
		},
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	result := ""
	if len(resp.Choices) > 0 {
		result = strings.TrimSpace(resp.Choices[0].Message.Content)
	}
	c.JSON(http.StatusOK, gin.H{
		"result": result,
		"model":  model,
		"usage":  resp.Usage,
	})
}
