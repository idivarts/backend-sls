package main

import (
	messagewebhook "github.com/idivarts/backend-sls/internal/message_webhook"
	sqsapp "github.com/idivarts/backend-sls/internal/message_webhook/sqs"
	apihandler "github.com/idivarts/backend-sls/pkg/api_handler"
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
