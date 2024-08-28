package main

import (
	messagewebhook "github.com/TrendsHub/th-backend/internal/message_webhook"
	sqsapp "github.com/TrendsHub/th-backend/internal/message_webhook/sqs"
	apihandler "github.com/TrendsHub/th-backend/pkg/api_handler"
)

func main() {
	webhooksHandler := apihandler.GinEngine.Group("/webhooks")

	webhooksHandler.POST("/instagram", messagewebhook.Receive)
	webhooksHandler.GET("/instagram", messagewebhook.Validation)

	webhooksHandler.POST("/facebook", messagewebhook.Receive)
	webhooksHandler.GET("/facebook", messagewebhook.Validation)

	apihandler.GinEngine.GET("/test/sqs", sqsapp.SendTestSQSMessage)

	apihandler.StartLambda()
}
