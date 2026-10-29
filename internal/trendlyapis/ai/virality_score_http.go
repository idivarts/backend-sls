package ai

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// scoreReq is the live editor state to score. It mirrors captionReq so the
// client sends the same shape it already builds for every other AI call.
type scoreReq struct {
	BrandID   string `json:"brandId" binding:"required"`
	ContextID string `json:"contextId"`
	Model     string `json:"model"`

	Script    string   `json:"script"`
	Format    string   `json:"format"`
	Platform  string   `json:"platform"`
	Platforms []string `json:"platforms"`
}

// HTTPScore scores a script against the virality rubric.
//
// HTTP rather than websocket because there is nothing to stream: five short
// judge calls run concurrently and the result is one structured object. The
// panel shows a spinner for a couple of seconds and then the whole score,
// which is a better shape than watching numbers trickle in.
func HTTPScore(c *gin.Context) {
	managerID, _ := middlewares.GetUserId(c)
	var req scoreReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !verifyBrandAccess(req.BrandID, managerID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	script := req.Script
	platforms := platformList(req.Platform, req.Platforms)
	format := req.Format

	// Fall back to the saved doc when the client sends no live state — scoring
	// a piece from the calendar, say, where there is no editor open.
	if script == "" && req.ContextID != "" {
		if ct, err := trendlymodels.GetContent(req.BrandID, req.ContextID); err == nil && ct != nil {
			script = ct.Script
			if format == "" {
				format = ct.ContentFormat
			}
			if len(platforms) == 0 {
				platforms = ct.Platforms
			}
		}
	}
	if script == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no script to score"})
		return
	}

	model, locked := pickModel(c.Request.Context(), req.BrandID, openrouter.TaskScore, req.Model)
	if locked {
		c.JSON(http.StatusPaymentRequired, gin.H{"error": "upgrade_required", "task": openrouter.TaskScore})
		return
	}
	orgID, _ := orgIDForBrand(req.BrandID)
	if aiTokensExhausted(orgID) {
		c.JSON(http.StatusPaymentRequired, gin.H{"error": "upgrade_required", "reason": "tokens_exhausted", "task": openrouter.TaskScore})
		return
	}

	score, usage, err := ScoreScript(c.Request.Context(), script, format, platforms, model)
	// Meter whatever was spent even when a dimension failed: the tokens left
	// the wallet either way, and silently absorbing them would make the
	// balance drift from reality.
	meterAIUsage(orgID, usage)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	score.ScoredAt = time.Now().UnixMilli()

	// Persist the latest score onto the content doc when we know which one it
	// is. Scoring costs five model calls, so a panel that re-scored every time
	// it opened would be charging the user to look at a number they already
	// have. Latest-only: the history that Phase 4's correlation needs is a
	// separate store, and keeping a growing array on the content doc would
	// bloat a document the editor reads constantly.
	//
	// Best-effort. A failed write means the next open re-scores, which is
	// wasteful but correct, and is not worth failing the request the user is
	// waiting on.
	if req.ContextID != "" {
		// Through FirestoreMap, not the struct: Firestore ignores `json:` tags
		// and would store the Go field names. See the note on that method.
		if doc, err := score.FirestoreMap(); err != nil {
			log.Printf("ai score encode brand=%s content=%s: %v", req.BrandID, req.ContextID, err)
		} else if err := trendlymodels.UpdateContentFields(req.BrandID, req.ContextID,
			map[string]any{"viralityScore": doc}); err != nil {
			log.Printf("ai score persist brand=%s content=%s: %v", req.BrandID, req.ContextID, err)
		}
	}

	c.JSON(http.StatusOK, gin.H{"score": score, "usage": usage})
}
