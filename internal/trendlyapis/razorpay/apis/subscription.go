package razorpayapis

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/myutil"
	"github.com/idivarts/backend-sls/pkg/payments"
)

type CreateSubscriptionRequest struct {
	BrandID      string `json:"brandId" binding:"required"`
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

	if brand.Billing == nil {
		trialDays = 0
	}

	_, link, err := payments.CreateSubscriptionLink(planId, billingCycle, trialDays, 0, map[string]interface{}{
		"brandId":      req.BrandID,
		"planName":     planName,
		"isGrowthPlan": req.IsGrowthPlan,
	}, "")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Unable to create subscription link"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully Created Subscription", "link": link})
}

type CancelSubscriptionRequest struct {
	BrandID string `json:"brandId" binding:"required"`
	Reason  string `json:"reason" binding:"required"`
	Note    string `json:"note"`
}

func CancelSubscription(c *gin.Context) {
	var req CancelSubscriptionRequest
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

	if brand.Billing == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "billing-not-defined", "message": "Billing is not defined"})
		return
	}

	data, err := payments.CancelSubscription(myutil.DerefString(brand.Billing.Subscription), (brand.Billing.BillingStatus != nil && *brand.Billing.BillingStatus == "active"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Not a part of the current brand"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully Cancelled Subscription", "data": data})
}
