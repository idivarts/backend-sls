package ai

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// These endpoints back the new full-page /onboarding flow's "what next" branch.
// They do all the setup a destination needs in one server round-trip — create
// the seed record, create the AI conversation, and persist the opening
// message(s) — so the destination screen can render fully formed with no
// further per-section loaders (the flow keeps its own loader up until this
// returns).

type initStrategyReq struct {
	BrandID string `json:"brandId" binding:"required"`
	Model   string `json:"model"`
}

// HTTPOnboardingStrategyInit creates an empty strategy, opens a strategy-scoped
// AI conversation, sends a pre-written kickoff message summarising the brand,
// and persists the assistant's first reply. Returns the ids the app routes to.
func HTTPOnboardingStrategyInit(c *gin.Context) {
	managerID, _ := middlewares.GetUserId(c)

	var req initStrategyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !verifyBrandAccess(req.BrandID, managerID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	ctx := c.Request.Context()
	brand, err := loadBrand(req.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "brand not found"})
		return
	}

	// 1. Create an empty strategy (mirrors the frontend toIStrategy shape so the
	//    strategies list + detail page render it identically to a hand-created one).
	now := time.Now().UnixMilli()
	stratRef, _, err := firestoredb.Client.
		Collection("brands").Doc(req.BrandID).
		Collection("strategies").
		Add(ctx, map[string]any{
			"name":            "My Content Strategy",
			"managerId":       managerID,
			"status":          "active",
			"markdownContent": "",
			"reviewStatus":    "draft",
			"createdAt":       now,
			"updatedAt":       now,
		})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	strategyID := stratRef.ID

	// 2. Create the strategy-scoped conversation.
	model, locked := pickModel(ctx, req.BrandID, openrouter.TaskChat, req.Model)
	if locked {
		c.JSON(http.StatusPaymentRequired, gin.H{"error": "upgrade_required", "task": openrouter.TaskChat})
		return
	}
	conv, err := openrouter.CreateConversation(ctx, req.BrandID, managerID, moduleStrategy, strategyID, model, "Content strategy")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 3. Compose the pre-written first message from the brand details and ask the
	//    AI to drive the strategy conversation.
	kickoff := buildStrategyKickoffMessage(brand)
	systemPrompt := buildSystemPrompt(brand, moduleStrategy, req.BrandID, strategyID, "")
	msgs := []openrouter.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: kickoff},
	}

	answer := ""
	if resp, cErr := openrouter.ChatCompletion(ctx, openrouter.ChatRequest{Model: model, Messages: msgs}); cErr == nil && len(resp.Choices) > 0 {
		answer = resp.Choices[0].Message.Content
	}

	// 4. Persist the opening turn so the panel shows it immediately on first load.
	_, _ = openrouter.AppendMessage(ctx, conv.ID, trendlymodels.AIMessage{
		Role: "user", Content: kickoff, Timestamp: time.Now().UnixMilli(),
	})
	if answer != "" {
		_, _ = openrouter.AppendMessage(ctx, conv.ID, trendlymodels.AIMessage{
			Role: "assistant", Content: answer, Model: model, Timestamp: time.Now().UnixMilli(),
		})
	}
	_ = openrouter.UpdateConversationModel(ctx, conv.ID, model)

	c.JSON(http.StatusOK, gin.H{
		"strategyId":     strategyID,
		"conversationId": conv.ID,
	})
}

type initCalendarReq struct {
	BrandID string `json:"brandId" binding:"required"`
	Model   string `json:"model"`
}

// HTTPOnboardingCalendarInit drops one template idea on today's date and opens
// the calendar AI conversation seeded with a fixed welcome message. The
// conversation uses contextId "calendar" to match the Content Calendar screen's
// single shared thread (CALENDAR_CONTEXT_ID on the client).
func HTTPOnboardingCalendarInit(c *gin.Context) {
	managerID, _ := middlewares.GetUserId(c)

	var req initCalendarReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !verifyBrandAccess(req.BrandID, managerID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	ctx := c.Request.Context()
	model, locked := pickModel(ctx, req.BrandID, openrouter.TaskChat, req.Model)
	if locked {
		c.JSON(http.StatusPaymentRequired, gin.H{"error": "upgrade_required", "task": openrouter.TaskChat})
		return
	}

	// 1. Create a template content item on today (midnight UTC, mirroring how the
	//    client places new calendar items).
	now := time.Now()
	todayTs := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).UnixMilli()
	nowMs := now.UnixMilli()
	contentRef, _, err := firestoredb.Client.
		Collection("brands").Doc(req.BrandID).
		Collection("contents").
		Add(ctx, map[string]any{
			"title":            "My first content idea",
			"managerId":        managerID,
			"description":      "A starter idea to show how your calendar works — edit it, or ask the AI to plan more.",
			"platform":         "Instagram",
			"contentFormat":    "post",
			"status":           "draft",
			"isArchived":       false,
			"postingTimeStamp": todayTs,
			"createdAt":        nowMs,
			"updatedAt":        nowMs,
		})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 2. Open the calendar conversation (contextId "calendar" — shared thread).
	conv, err := openrouter.CreateConversation(ctx, req.BrandID, managerID, moduleCalendar, "calendar", model, "Content calendar")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 3. Seed a fixed assistant welcome (deterministic — no model call needed).
	intro := "I've created your first test idea on the calendar. Ask me anything — for example, add a new idea, or edit or delete one — and I'll help you with it."
	_, _ = openrouter.AppendMessage(ctx, conv.ID, trendlymodels.AIMessage{
		Role: "assistant", Content: intro, Timestamp: time.Now().UnixMilli(),
	})

	c.JSON(http.StatusOK, gin.H{
		"conversationId": conv.ID,
		"contentId":      contentRef.ID,
	})
}

// buildStrategyKickoffMessage composes the pre-written first user turn that
// summarises the brand and asks the AI to lead the strategy conversation.
func buildStrategyKickoffMessage(brand *trendlymodels.Brand) string {
	var sb strings.Builder
	sb.WriteString("Hi! I'd love your help building a content strategy for my brand. Here's what you should know:\n")
	if brand != nil {
		if strings.TrimSpace(brand.Name) != "" {
			sb.WriteString("- Brand name: " + brand.Name + "\n")
		}
		if brand.Profile != nil && brand.Profile.About != nil && strings.TrimSpace(*brand.Profile.About) != "" {
			sb.WriteString("- About: " + *brand.Profile.About + "\n")
		}
		if brand.Age != nil && strings.TrimSpace(*brand.Age) != "" {
			sb.WriteString("- How established we are: " + brandAgeLabel(*brand.Age) + "\n")
		}
		if brand.Profile != nil && brand.Profile.Website != nil && strings.TrimSpace(*brand.Profile.Website) != "" {
			sb.WriteString("- Website: " + *brand.Profile.Website + "\n")
		}
	}
	sb.WriteString("\nPlease ask me the questions that would help you form a strong content strategy for me.")
	return sb.String()
}

// brandAgeLabel maps the stored brand-age bucket to a human phrase for the prompt.
func brandAgeLabel(age string) string {
	switch age {
	case "JUST_STARTING":
		return "just starting / pre-launch"
	case "LT_1":
		return "less than a year old"
	case "LT_5":
		return "less than 5 years old"
	case "GT_5":
		return "5+ years old"
	default:
		return age
	}
}
