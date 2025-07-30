package main

import (
	"github.com/idivarts/backend-sls/internal/middlewares"
	razorpayapis "github.com/idivarts/backend-sls/internal/trendlyapis/razorpay/apis"
	apihandler "github.com/idivarts/backend-sls/pkg/api_handler"
)

func main() {
	handler := apihandler.GinEngine.Group("/razorpay", middlewares.ValidateSessionMiddleware(), middlewares.TrendlyMiddleware("managers"))

	handler.POST("/create-subscription", razorpayapis.CreateSubscription)
	handler.POST("/cancel-subscription", razorpayapis.CancelSubscription)

	handler.POST("/collaborations/boost", razorpayapis.CollaborationBoost)
	handler.POST("/collaborations/handle", razorpayapis.CollaborationHandleSupport)

	apihandler.StartLambda()
}
