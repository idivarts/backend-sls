package main

import (
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	razorpayapis "github.com/idivarts/backend-sls/internal/trendlyapis/razorpay/apis"
	apihandler "github.com/idivarts/backend-sls/pkg/api_handler"
)

func main() {
	handler := apihandler.GinEngine.Group("/monetize", middlewares.ValidateSessionMiddleware())

	brandAPIs(handler)
	influencersAPIs(handler)

	apihandler.StartLambda()
}

func brandAPIs(handler *gin.RouterGroup) {
	brands := handler.Group("/brands", middlewares.TrendlyMiddleware("brands"))

	brands.POST("/contracts/:contractId/create-order", razorpayapis.CreateSubscription)
}

func influencersAPIs(handler *gin.RouterGroup) {
	influencer := handler.Group("/influencers", middlewares.TrendlyMiddleware("users"))

	influencer.POST("/create-account", razorpayapis.CreateSubscription)
}
