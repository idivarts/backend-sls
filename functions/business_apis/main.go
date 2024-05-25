package main

import (
	businessapis "github.com/TrendsHub/th-backend/internal/business_apis"
	apihandler "github.com/TrendsHub/th-backend/pkg/api_handler"
)

func main() {
	apihandler.GinEngine.POST("/business/login", businessapis.Login)
	apihandler.GinEngine.GET("/business/pages", businessapis.GetPages)

	// All newly changed apis below
	apihandler.GinEngine.POST("/business/pages/:pageId/webhook", businessapis.PageWebhook)
	apihandler.GinEngine.POST("/business/pages/:pageId/assistant", businessapis.PageAssistant)
	apihandler.GinEngine.POST("/business/pages/:pageId/sync", businessapis.PageSync)

	apihandler.GinEngine.GET("/business/conversations", businessapis.GetConversations)

	apihandler.GinEngine.GET("/business/conversations/:igsid", businessapis.GetConversationById)
	apihandler.GinEngine.PUT("/business/conversations/:igsid", businessapis.UpdateConversation)

	apihandler.GinEngine.GET("/business/messages/:igsid", businessapis.GetMessages)
	apihandler.GinEngine.POST("/business/messages/:igsid", businessapis.SendMessage)

	apihandler.StartLambda()
}
