package ai

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

type hooksReq struct {
	BrandID   string `json:"brandId" binding:"required"`
	ContextID string `json:"contextId"`
	Model     string `json:"model"`

	Topic     string   `json:"topic"`
	Title     string   `json:"title"`
	Script    string   `json:"script"`
	Format    string   `json:"format"`
	Platform  string   `json:"platform"`
	Platforms []string `json:"platforms"`
}

// HTTPHooks returns five ranked opening hooks for a script.
//
// HTTP for the same reason as scoring: two model calls produce one structured
// list, and there is nothing worth streaming. Nothing is persisted — these are
// candidates to choose between, and the one the user picks becomes part of the
// script through the editor like any other edit.
func HTTPHooks(c *gin.Context) {
	managerID, _ := middlewares.GetUserId(c)
	var req hooksReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !verifyBrandAccess(req.BrandID, managerID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	brief := hookBrief{
		Topic:     req.Topic,
		Title:     req.Title,
		Body:      req.Script,
		Format:    req.Format,
		Platforms: platformList(req.Platform, req.Platforms),
	}

	// Same saved-doc fallback as scoring, so a piece can be laddered from the
	// calendar without an editor open.
	if brief.Body == "" && req.ContextID != "" {
		if ct, err := trendlymodels.GetContent(req.BrandID, req.ContextID); err == nil && ct != nil {
			brief.Body = ct.Script
			if brief.Title == "" {
				brief.Title = ct.Title
			}
			if brief.Topic == "" {
				brief.Topic = ct.Description
			}
			if brief.Format == "" {
				brief.Format = ct.ContentFormat
			}
			if len(brief.Platforms) == 0 {
				brief.Platforms = ct.Platforms
			}
		}
	}

	model, locked := pickModel(c.Request.Context(), req.BrandID, openrouter.TaskHooks, req.Model)
	if locked {
		c.JSON(http.StatusPaymentRequired, gin.H{"error": "upgrade_required", "task": openrouter.TaskHooks})
		return
	}
	orgID, _ := orgIDForBrand(req.BrandID)
	if aiTokensExhausted(orgID) {
		c.JSON(http.StatusPaymentRequired, gin.H{"error": "upgrade_required", "reason": "tokens_exhausted", "task": openrouter.TaskHooks})
		return
	}

	ladder, usage, err := BuildHookLadder(c.Request.Context(), brief, model)
	// Meter first: both calls spent tokens whether or not the second parsed.
	meterAIUsage(orgID, usage)
	if err != nil {
		// Ranking can fail with the hooks intact. Unranked hooks are still the
		// useful part of this feature, so they are returned with a note rather
		// than thrown away — the client shows them in generated order.
		if ladder != nil && len(ladder.Hooks) > 0 {
			ladder.GeneratedAt = time.Now().UnixMilli()
			c.JSON(http.StatusOK, gin.H{"ladder": ladder, "usage": usage, "warning": err.Error()})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	ladder.GeneratedAt = time.Now().UnixMilli()

	c.JSON(http.StatusOK, gin.H{"ladder": ladder, "usage": usage})
}
