package main

import (
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/trendlyapis/monetize"
	apihandler "github.com/idivarts/backend-sls/pkg/api_handler"
)

func main() {
	handler := apihandler.GinEngine.Group("/monetize", middlewares.ValidateSessionMiddleware())

	brandAPIs(handler)
	influencersAPIs(handler)
	webhookHandler(handler)

	apihandler.StartLambda()
}

func brandAPIs(handler *gin.RouterGroup) {
	brands := handler.Group("/brands", middlewares.TrendlyMiddleware("brands"))

	// [BRAND] API to create payment order - for pre-payment of contract
	brands.POST("/contracts/:contractId/order", monetize.Placeholder)
	// [BRAND] API to check the payment status of the order
	brands.GET("/contracts/:contractId/order", monetize.Placeholder)

	// [BRAND] API for marking as shipped
	brands.POST("/contracts/:contractId/shipment", monetize.Placeholder)
	// [BRANDS] API for mark as product delivered
	brands.POST("/contracts/:contractId/shipment/delivered", monetize.Placeholder)

	// [BRAND] Request for First Video/ Revision
	brands.POST("/contracts/:contractId/deliverable/request", monetize.Placeholder)
	// [BRAND] Approve the Video
	brands.POST("/contracts/:contractId/deliverable/approve", monetize.Placeholder)

	// [BRAND] Schedule/Reschedule the release date of the video
	brands.POST("/contracts/:contractId/posting/schedule", monetize.Placeholder)
}

func influencersAPIs(handler *gin.RouterGroup) {
	influencer := handler.Group("/influencers", middlewares.TrendlyMiddleware("users"))

	// API for creating/re-submit a Razorpay Route account
	influencer.POST("/account", monetize.Placeholder)

	// [User Polling] For checking if the account is approved or still in needs clarification
	influencer.GET("/account", monetize.Placeholder)

	// API to update the Bank Account
	influencer.POST("/account/bank", monetize.Placeholder)

	// API to update the Shipping Address
	influencer.POST("/account/address", monetize.Placeholder)

	// [USER] API for Requesting brands to ship the product
	influencer.POST("/contracts/:contractId/shipment/request", monetize.Placeholder)
	// [USER] API for mark as product received
	influencer.POST("/contracts/:contractId/shipment/received", monetize.Placeholder)

	// [User] Submit the First Video/ Revision
	influencer.POST("/contracts/:contractId/deliverable", monetize.Placeholder)
	// [USER] Request for Approval
	influencer.POST("/contracts/:contractId/deliverable/request-approval", monetize.Placeholder)

	// [USER] Request to (Re)Schedule a release
	influencer.POST("/contracts/:contractId/posting/request-schedule", monetize.Placeholder)
	// [USER] Mark video as Posted
	influencer.POST("/contracts/:contractId/posting", monetize.Placeholder)
}

func webhookHandler(handler *gin.RouterGroup) {
	webhook := handler.Group("/webhooks")

	// [BRAND Webhook] Listen to check the payment status and mark the collaboration paid
	webhook.Any("/payments", monetize.Placeholder)

	// Once Payment processed, notify both the agents and close the contract
	webhook.Any("/tranfer", monetize.Placeholder)

}

// Remind User and Brands on the posting day (Multiple Reminders needed)
