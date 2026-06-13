package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// validContentFormats are the formats the content calendar understands (it casts
// IContent.contentFormat straight to its lowercase ContentType union).
var validContentFormats = map[string]bool{
	"reel": true, "post": true, "story": true, "carousel": true, "live": true,
}

// Strategy lifecycle status values written to a strategy's `status` field.
// Mirrors the StrategyStatus enum in the frontend shared-libs model
// (trendly-pro/models/strategies.ts) — keep the two in sync.
const (
	StrategyStatusActive    = "active"
	StrategyStatusFinalized = "finalized" // pushed to calendar → doc + chat locked
)

// ── Push to Calendar ─────────────────────────────────────────────────────────

type pushToCalendarReq struct {
	BrandID          string `json:"brandId" binding:"required"`
	StartDate        string `json:"startDate" binding:"required"` // YYYY-MM-DD
	DurationDays     int    `json:"durationDays"`
	OverrideExisting bool   `json:"overrideExisting"`
}

type generatedItem struct {
	Title         string `json:"title"`
	Platform      string `json:"platform"`
	ContentFormat string `json:"contentFormat"`
	Description   string `json:"description"`
	DayOffset     int    `json:"dayOffset"`
}

// pushProgressFn receives staged progress while a push-to-calendar job runs.
// The HTTP path passes a no-op; the WebSocket path streams these to the client.
// `extra` carries optional structured fields (e.g. total/index/title).
type pushProgressFn func(phase, message string, extra map[string]any)

// pushResult is the outcome of a push-to-calendar run, shared by both transports.
type pushResult struct {
	CreatedItemIds []string
	RemovedItemIds []string
	StartDateStr   string
	EndDateStr     string
}

// runPushToCalendar is the shared core that expands a finalized strategy into
// scheduled content items under brands/{brandId}/contents, placed within
// [startDate, startDate+duration). The heavy step (generateCalendarItems) is an
// AI call that can exceed the API Gateway 30s HTTP limit, so the WebSocket path
// runs this with a progress callback while HTTP passes nil.
//
// Errors are returned with stable messages so the HTTP wrapper can map them to
// status codes: "startDate must be YYYY-MM-DD", "strategy not found",
// "strategy has no content to convert".
func runPushToCalendar(
	ctx context.Context,
	brandID, strategyID, managerID, startDate string,
	durationDays int,
	overrideExisting bool,
	progress pushProgressFn,
) (*pushResult, error) {
	if progress == nil {
		progress = func(string, string, map[string]any) {}
	}

	startMs, err := parseStartDateMs(startDate)
	if err != nil {
		return nil, fmt.Errorf("startDate must be YYYY-MM-DD")
	}
	duration := durationDays
	if duration <= 0 {
		duration = 30
	}
	endExclusiveMs := startMs + int64(duration)*dayMs

	progress("reading", "Reading your strategy…", nil)
	strat, err := trendlymodels.GetStrategy(ctx, brandID, strategyID)
	if err != nil {
		return nil, fmt.Errorf("strategy not found")
	}
	html := strat.MarkdownContent
	if strings.TrimSpace(html) == "" {
		return nil, fmt.Errorf("strategy has no content to convert")
	}

	progress("planning", "Designing your posting schedule with AI…", nil)
	items, err := generateCalendarItems(ctx, brandID, html, duration)
	if err != nil {
		return nil, err
	}

	// Replace existing: delete every content item whose date falls in the window.
	removedItemIds := []string{}
	if overrideExisting {
		progress("clearing", "Clearing existing items in the window…", nil)
		removedItemIds, _ = trendlymodels.DeleteContentInRange(ctx, brandID, startMs, endExclusiveMs)
	}

	progress("scheduling", fmt.Sprintf("Scheduling %d posts…", len(items)), map[string]any{"total": len(items)})

	now := time.Now().UnixMilli()
	createdItemIds := []string{}
	for idx, it := range items {
		off := it.DayOffset
		if off < 0 {
			off = 0
		}
		if off > duration-1 {
			off = duration - 1
		}
		format := strings.ToLower(strings.TrimSpace(it.ContentFormat))
		if !validContentFormats[format] {
			format = "post"
		}
		platform := strings.TrimSpace(it.Platform)
		if platform == "" {
			platform = "Instagram"
		}
		title := strings.TrimSpace(it.Title)
		id, e := trendlymodels.CreateContent(ctx, brandID, map[string]any{
			"title":            title,
			"managerId":        managerID,
			"strategyId":       strategyID,
			"platform":         platform,
			"contentFormat":    format,
			"status":           "draft",
			"description":      strings.TrimSpace(it.Description),
			"postingTimeStamp": startMs + int64(off)*dayMs,
			"isArchived":       false,
			"createdAt":        now,
			"updatedAt":        now,
		})
		if e == nil {
			createdItemIds = append(createdItemIds, id)
			progress("item", "Scheduled: "+title, map[string]any{
				"index": idx + 1,
				"total": len(items),
				"title": title,
			})
		}
	}

	// A successful push finalizes the strategy: the document is now driving live
	// scheduled content, so the apps lock it (read-only doc + chat) and offer a
	// "Duplicate" path for further iteration. Best-effort — a failed status write
	// shouldn't fail the push the user already saw succeed.
	progress("finalizing", "Finalizing your strategy…", nil)
	_ = trendlymodels.UpdateStrategy(ctx, brandID, strategyID, []firestore.Update{
		{Path: "status", Value: StrategyStatusFinalized},
		{Path: "updatedAt", Value: time.Now().UnixMilli()},
	})

	return &pushResult{
		CreatedItemIds: createdItemIds,
		RemovedItemIds: removedItemIds,
		StartDateStr:   startDate,
		EndDateStr:     msToDateStr(startMs + int64(duration-1)*dayMs),
	}, nil
}

// HTTPPushToCalendar is the synchronous transport over runPushToCalendar. Kept
// as a fallback; the apps drive this over the WebSocket (see handlePushToCalendarWS)
// because the AI step can exceed the 30s HTTP limit.
func HTTPPushToCalendar(c *gin.Context) {
	strategyID := c.Param("strategyId")
	managerID, _ := middlewares.GetUserId(c)

	var req pushToCalendarReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !verifyBrandAccess(req.BrandID, managerID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	res, err := runPushToCalendar(
		c.Request.Context(), req.BrandID, strategyID, managerID,
		req.StartDate, req.DurationDays, req.OverrideExisting, nil,
	)
	if err != nil {
		switch err.Error() {
		case "startDate must be YYYY-MM-DD", "strategy has no content to convert":
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case "strategy not found":
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"strategyId":     strategyID,
		"createdItemIds": res.CreatedItemIds,
		"removedItemIds": res.RemovedItemIds,
		"startDate":      res.StartDateStr,
		"endDate":        res.EndDateStr,
	})
}

func generateCalendarItems(ctx context.Context, brandID, html string, duration int) ([]generatedItem, error) {
	sys := "You convert a content-strategy document into a concrete posting schedule. " +
		"Read the strategy and produce one content item per planned post across the campaign. " +
		"Spread items sensibly over the window using dayOffset (0-based, from 0 to " +
		fmt.Sprintf("%d", duration-1) + " inclusive). contentFormat MUST be one of: reel, post, story, carousel, live. " +
		"description is a short idea-level brief for the post. " +
		"Respond with ONLY a JSON object of the form " +
		`{"items":[{"title":string,"platform":string,"contentFormat":string,"description":string,"dayOffset":number}]} ` +
		"and nothing else."
	user := fmt.Sprintf("Campaign length: %d days.\n\nStrategy document (HTML):\n%s", duration, html)

	model, locked := pickModel(ctx, brandID, openrouter.TaskStrategy, "")
	if locked {
		return nil, fmt.Errorf("upgrade_required")
	}
	orgID, _ := orgIDForBrand(brandID)
	if aiTokensExhausted(orgID) {
		return nil, fmt.Errorf("upgrade_required")
	}
	resp, err := openrouter.ChatCompletion(ctx, openrouter.ChatRequest{
		Model:          model,
		ResponseFormat: &openrouter.ResponseFormat{Type: "json_object"},
		Messages: []openrouter.Message{
			{Role: "system", Content: sys},
			{Role: "user", Content: user},
		},
	})
	if err != nil {
		return nil, err
	}
	meterAIUsage(orgID, resp.Usage)
	raw := ""
	if len(resp.Choices) > 0 {
		raw = resp.Choices[0].Message.Content
	}
	var parsed struct {
		Items []generatedItem `json:"items"`
	}
	if err := json.Unmarshal([]byte(extractJSON(raw)), &parsed); err != nil {
		return nil, fmt.Errorf("could not parse strategy items")
	}
	return parsed.Items, nil
}

// ── Re-check Duration ────────────────────────────────────────────────────────

type recheckDurationReq struct {
	BrandID string `json:"brandId" binding:"required"`
}

// HTTPRecheckDuration re-reads the strategy body and re-derives the campaign
// length, persisting the corrected window back onto the strategy timeline.
func HTTPRecheckDuration(c *gin.Context) {
	strategyID := c.Param("strategyId")
	managerID, _ := middlewares.GetUserId(c)

	var req recheckDurationReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !verifyBrandAccess(req.BrandID, managerID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	ctx := c.Request.Context()
	strat, err := trendlymodels.GetStrategy(ctx, req.BrandID, strategyID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "strategy not found"})
		return
	}
	html := strat.MarkdownContent
	if strings.TrimSpace(html) == "" {
		c.JSON(http.StatusOK, gin.H{"strategyId": strategyID, "durationDays": nil})
		return
	}

	days, confidence := deriveDuration(ctx, req.BrandID, html)
	if days <= 0 {
		c.JSON(http.StatusOK, gin.H{"strategyId": strategyID, "durationDays": nil, "confidence": confidence})
		return
	}

	// Persist the corrected window — keep the existing startDate if present.
	startMs := time.Now().UnixMilli()
	if strat.Timeline != nil && strat.Timeline.StartDate > 0 {
		startMs = strat.Timeline.StartDate
	}
	_ = trendlymodels.UpdateStrategy(ctx, req.BrandID, strategyID, []firestore.Update{
		{Path: "timeline", Value: trendlymodels.StrategyTimeline{StartDate: startMs, EndDate: startMs + int64(days)*dayMs}},
		{Path: "updatedAt", Value: time.Now().UnixMilli()},
	})

	c.JSON(http.StatusOK, gin.H{"strategyId": strategyID, "durationDays": days, "confidence": confidence})
}

func deriveDuration(ctx context.Context, brandID, html string) (int, float64) {
	sys := "Read the content-strategy document and determine how many days the campaign is intended to run. " +
		"Respond with ONLY a JSON object {\"durationDays\":number,\"confidence\":number} where confidence is 0–1. " +
		"If the document does not clearly state or imply a length, set durationDays to 0."
	model, locked := pickModel(ctx, brandID, openrouter.TaskStrategy, "")
	if locked {
		return 0, 0
	}
	orgID, _ := orgIDForBrand(brandID)
	if aiTokensExhausted(orgID) {
		return 0, 0
	}
	resp, err := openrouter.ChatCompletion(ctx, openrouter.ChatRequest{
		Model:          model,
		ResponseFormat: &openrouter.ResponseFormat{Type: "json_object"},
		Messages: []openrouter.Message{
			{Role: "system", Content: sys},
			{Role: "user", Content: "Strategy document (HTML):\n" + html},
		},
	})
	if err != nil || len(resp.Choices) == 0 {
		return 0, 0
	}
	meterAIUsage(orgID, resp.Usage)
	var parsed struct {
		DurationDays float64 `json:"durationDays"`
		Confidence   float64 `json:"confidence"`
	}
	if err := json.Unmarshal([]byte(extractJSON(resp.Choices[0].Message.Content)), &parsed); err != nil {
		return 0, 0
	}
	return int(parsed.DurationDays), parsed.Confidence
}

// ── helpers ──────────────────────────────────────────────────────────────────

func parseStartDateMs(s string) (int64, error) {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(s))
	if err != nil {
		return 0, err
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).UnixMilli(), nil
}

func msToDateStr(ms int64) string {
	return time.UnixMilli(ms).UTC().Format("2006-01-02")
}

// extractJSON pulls the JSON object out of a model reply, tolerating ```json
// fences or stray prose around it.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}
