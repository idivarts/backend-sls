package razorpayapis

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"firebase.google.com/go/v4/auth"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	paymentwebhooks "github.com/idivarts/backend-sls/internal/trendlyapis/razorpay/payment_webhooks"
	"github.com/idivarts/backend-sls/pkg/firebase/fauth"
	"github.com/idivarts/backend-sls/pkg/myutil"
	"github.com/idivarts/backend-sls/pkg/payments"
)

type CreateSubscriptionRequestV2 struct {
	BrandID   string `json:"brandId" binding:"required"`
	PlanKey   string `json:"planKey" binding:"required"`
	PlanCycle string `json:"planCycle" binding:"required"`

	AdminData *AdminData `json:"adminData,omitempty"`
}
type AdminData struct {
	IsOnTrial      bool    `json:"isOnTrial"`
	TrialDays      int     `json:"trialDays"`
	Email          string  `json:"email"`
	Password       string  `json:"password"`
	Phone          string  `json:"phone"`
	OfferId        *string `json:"offerId,omitempty"`
	OneTimePayment *int    `json:"oneTimePayment,omitempty"`
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

	body, err := payments.Client.Plan.All(map[string]interface{}{
		"from": PLAN_LAST_TIME,
	}, nil)
	if plans, ok := body["items"].([]interface{}); ok {
		for _, item := range plans {
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

	userExists := true
	offerId := ""
	if req.AdminData != nil {
		if req.AdminData.IsOnTrial {
			if req.AdminData.TrialDays <= 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "trial-error", "message": "Trial Days should be atleast for one day"})
				return
			}
			trialDays = req.AdminData.TrialDays
		}
		if req.AdminData.OfferId != nil {
			offerId = *req.AdminData.OfferId
		}

		email := req.AdminData.Email
		password := req.AdminData.Password

		userRecord, err := fauth.Client.GetUserByEmail(context.Background(), email)
		if err != nil {
			userExists = false
			user := &auth.UserToCreate{}
			user = user.Email(email).Password(password).EmailVerified(true)
			userRecord, err = fauth.Client.CreateUser(context.Background(), user)
		}
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error creating or getting user"})
			return
		}

		uid := userRecord.UID

		manager := trendlymodels.Manager{}
		err = manager.Get(uid)
		if err != nil {
			manager := trendlymodels.Manager{
				Name:         userRecord.DisplayName,
				Email:        userRecord.Email,
				IsAdmin:      false,
				ProfileImage: userRecord.PhotoURL,
				CreationTime: time.Now().UnixMilli(),
			}
			_, err = manager.Insert(uid)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error inserting Manager"})
				return
			}
		}

		brandMember := trendlymodels.BrandMember{
			ManagerID: uid,
			Role:      "manager",
			Status:    1,
		}
		_, err = brandMember.Set(req.BrandID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error inserting Brand Manager Entry"})
			return
		}
	}

	notes := map[string]interface{}{
		"brandId":   req.BrandID,
		"planKey":   planKey,
		"planCycle": planCycle,
	}

	var id, link string

	if req.AdminData != nil && req.AdminData.OneTimePayment != nil {
		id, link, err = payments.CreatePaymentLink(700, payments.Customer{
			Email:       req.AdminData.Email,
			PhoneNumber: req.AdminData.Phone,
		}, notes)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Unable to create one time payment link"})
			return
		}
	} else {
		id, link, err = payments.CreateSubscriptionLink(planId, billingCycle, trialDays, 0, notes, offerId)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Unable to create subscription link"})
			return
		}
	}

	if req.AdminData != nil && req.AdminData.IsOnTrial {
		tEndTime := time.Now().Add(time.Duration(trialDays * 24 * int(time.Hour))).UnixMilli()
		brand.Billing = &trendlymodels.BrandBilling{
			Subscription:    &id,
			SubscriptionUrl: &link,
			BillingStatus:   myutil.StrPtr("created"),
			PlanKey:         &planKey,
			PlanCycle:       &planCycle,
			IsOnTrial:       myutil.BoolPtr(true),
			TrialEnds:       &tEndTime,
			Status:          myutil.IntPtr(0),
		}
		x, b := trendlymodels.PlanCreditsMap[planKey]
		if b {
			brand.Credits = x
		}

		if req.AdminData != nil && req.AdminData.OneTimePayment != nil {
			brand.Billing.Subscription = nil
			brand.Billing.PaymentLinkId = &id
		}

		brand.HasPayWall = true

		_, err = brand.Insert(req.BrandID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error updating Brand Subscription"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully Created Subscription",
		"link":       link,
		"userExists": req.AdminData != nil && userExists,
	})
}

func UpdateSubscription(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"message": "Update Subscription V2 is not implemented yet"})
}
