package main

import (
	businessapis "github.com/TrendsHub/th-backend/internal/business_apis"
	apihandler "github.com/TrendsHub/th-backend/pkg/api_handler"
)

func main() {
	apihandler.GinEngine.POST("/business/login", businessapis.Login)
	apihandler.GinEngine.POST("/business/pages", businessapis.GetPages)

	apihandler.StartLambda()
}
