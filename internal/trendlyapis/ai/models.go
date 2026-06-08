package ai

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

type modelListItem struct {
	openrouter.ModelInfo
	Unlocked bool `json:"unlocked"`
}

func ListModels(c *gin.Context) {
	managerID, _ := middlewares.GetUserId(c)
	brandID := c.Query("brandId")
	tier := openrouter.TierStarter
	if brandID != "" && verifyBrandAccess(brandID, managerID) {
		if brand, err := loadBrand(brandID); err == nil && brand.OrganizationID != nil && *brand.OrganizationID != "" {
			org := &trendlymodels.Organization{}
			if err := org.Get(*brand.OrganizationID); err == nil && org.Billing != nil && org.Billing.PlanKey != nil {
				tier = openrouter.TierFromPlanKey(*org.Billing.PlanKey)
			}
		}
	}
	out := make([]modelListItem, 0, len(openrouter.Models))
	for _, m := range openrouter.Models {
		out = append(out, modelListItem{
			ModelInfo: m,
			Unlocked:  openrouter.IsUnlockedFor(m, tier),
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"tier":   tier,
		"models": out,
	})
}
