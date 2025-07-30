package razorpayapis

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/payments"
)

type CreateSubscriptionRequest struct {
	BrandID      string `json:"brandId,required"`
	IsGrowthPlan bool   `json:"isGrowthPlan"`
}

func CreateSubscription(c *gin.Context) {
	var req CreateSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid request payload"})
		return
	}

	userId, b := middlewares.GetUserId(c)
	if !b {
		c.JSON(http.StatusBadRequest, gin.H{"error": "manager-id-missing", "message": "No Manager ID found"})
		return
	}

	brand := &trendlymodels.Brand{}
	err := brand.Get(req.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid Brand ID"})
		return
	}

	brandMember := &trendlymodels.BrandMember{}
	err = brandMember.Get(req.BrandID, userId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Not a part of the current brand"})
		return
	}

	billingCycle := 12
	trialDays := 3
	planName := "Growth Plan"
	planId := GROWTH_PLAN_ID

	if !req.IsGrowthPlan {
		billingCycle = 2
		planName = "Business Plan"
		planId = BUSINESS_PLAN_ID
	}

	payments.CreateSubscriptionLink(planId, billingCycle, trialDays, 1, map[string]interface{}{
		"brandId":      req.BrandID,
		"planName":     planName,
		"isGrowthPlan": req.IsGrowthPlan,
	})
}

func CancelSubscription(c *gin.Context) {

}
