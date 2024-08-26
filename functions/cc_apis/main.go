package main

import (
	"github.com/TrendsHub/th-backend/internal/ccapis"
	apihandler "github.com/TrendsHub/th-backend/pkg/api_handler"
)

func main() {
	apihandler.GinEngine.POST("/business/login", ccapis.Login)
	apihandler.GinEngine.GET("/business/pages", ccapis.GetPages)

	// All newly changed apis below
	apihandler.GinEngine.POST("/business/pages/:pageId/webhook", ccapis.PageWebhook)
	apihandler.GinEngine.POST("/business/pages/:pageId/assistant", ccapis.PageAssistant)
	apihandler.GinEngine.POST("/business/pages/:pageId/sync", ccapis.PageSync)

	apihandler.GinEngine.GET("/business/conversations", ccapis.GetConversations)

	apihandler.GinEngine.GET("/business/conversations/:igsid", ccapis.GetConversationById)
	apihandler.GinEngine.PUT("/business/conversations/:igsid", ccapis.UpdateConversation)

	apihandler.GinEngine.GET("/business/messages/:igsid", ccapis.GetMessages)
	apihandler.GinEngine.POST("/business/messages/:igsid", ccapis.SendMessage)

	apihandler.StartLambda()
}
