package ai

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

type modelListItem struct {
	openrouter.ModelInfo
	Unlocked bool `json:"unlocked"`
}

// ListModels returns the model catalog with per-model unlock flags for the
// brand's current plan. The brand app primarily reads ai_config directly from
// Firestore; this endpoint is kept for backward-compat / server-side callers.
func ListModels(c *gin.Context) {
	managerID, _ := middlewares.GetUserId(c)
	brandID := c.Query("brandId")

	openrouter.EnsureRegistry(c.Request.Context())

	plan := openrouter.PlanFree
	if brandID != "" && verifyBrandAccess(brandID, managerID) {
		plan = brandPlan(brandID)
	}

	models := openrouter.ListModels()
	out := make([]modelListItem, 0, len(models))
	for _, m := range models {
		out = append(out, modelListItem{
			ModelInfo: m,
			Unlocked:  openrouter.Unlocked(m, plan),
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"plan":   plan,
		"models": out,
	})
}
