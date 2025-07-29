package main

import (
	"github.com/idivarts/backend-sls/internal/middlewares"
	razorpayapis "github.com/idivarts/backend-sls/internal/trendlyapis/razorpay/apis"
	apihandler "github.com/idivarts/backend-sls/pkg/api_handler"
)

func main() {
	handler := apihandler.GinEngine.Group("/razorpay", middlewares.ValidateSessionMiddleware(), middlewares.TrendlyMiddleware("managers"))

	handler.GET("/", razorpayapis.Test)

	apihandler.StartLambda()
}
