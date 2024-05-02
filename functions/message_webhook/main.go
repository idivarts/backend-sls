package main

import (
	messagewebhook "github.com/TrendsHub/th-backend/internal/message_webhook"
	sqsapp "github.com/TrendsHub/th-backend/internal/message_webhook/sqs"
	apihandler "github.com/TrendsHub/th-backend/pkg/api_handler"
)

func main() {
	apihandler.GinEngine.POST("/instagram/webhook", messagewebhook.Receive)
	apihandler.GinEngine.GET("/instagram/webhook", messagewebhook.Validation)
	apihandler.GinEngine.GET("/test/sqs", sqsapp.SendTestSQSMessage)

	apihandler.StartLambda()
}
