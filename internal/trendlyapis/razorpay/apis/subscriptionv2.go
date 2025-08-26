package razorpayapis

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	paymentwebhooks "github.com/idivarts/backend-sls/internal/trendlyapis/razorpay/payment_webhooks"
	"github.com/idivarts/backend-sls/pkg/payments"
)

type CreateSubscriptionRequestV2 struct {
	BrandID   string `json:"brandId" binding:"required"`
	PlanKey   string `json:"planKey" binding:"required"`
	PlanCycle string `json:"planCycle" binding:"required"`
}

func CreateSubscriptionV2(c *gin.Context) {
	var req CreateSubscriptionRequestV2
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
	trialDays := 0
	planKey := req.PlanKey
	planCycle := req.PlanCycle

	if req.PlanCycle == "yearly" {
		billingCycle = 5
	}

	planId := "" //payments.Plans[planKey+":"+planCycle]

	plans, err := payments.Client.Plan.All(map[string]interface{}{
		"from": PLAN_LAST_TIME,
	}, nil)
	if items, ok := plans["items"].([]interface{}); ok {
		for _, item := range items {
			var plan paymentwebhooks.PlanEntity
			b, _ := json.Marshal(item)   // convert map[string]interface{} -> []byte
			_ = json.Unmarshal(b, &plan) // convert []byte -> struct
			if plan.Notes.PlanKey == planKey && plan.Notes.PlanCycle == planCycle && plan.Notes.PlanVersion == PLAN_VERSION {
				planId = plan.ID
				break
			}
		}
	}

	if planId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid-plan", "message": "Invalid plan key or cycle"})
		return
	}

	if brand.Billing == nil {
		trialDays = 0
	}

	link, err := payments.CreateSubscriptionLink(planId, billingCycle, trialDays, 0, map[string]interface{}{
		"brandId":   req.BrandID,
		"planKey":   planKey,
		"planCycle": planCycle,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Unable to create subscription link"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully Created Subscription", "link": link})
}

func UpdateSubscription(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"message": "Update Subscription V2 is not implemented yet"})
}
