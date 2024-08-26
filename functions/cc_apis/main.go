package main

import (
	"github.com/TrendsHub/th-backend/internal/ccapis"
	apihandler "github.com/TrendsHub/th-backend/pkg/api_handler"
)

func main() {
	apihandler.GinEngine.GET("/sources", ccapis.GetPages)
	apihandler.GinEngine.POST("/sources/facebook", ccapis.FacebookLogin)

	// All newly changed apis below
	apihandler.GinEngine.POST("/sources/:pageId/webhook", ccapis.PageWebhook)
	apihandler.GinEngine.POST("/sources/:pageId/assistant", ccapis.PageAssistant)
	apihandler.GinEngine.POST("/sources/:pageId/sync", ccapis.PageSync)

	apihandler.GinEngine.GET("/conversations", ccapis.GetConversations)

	apihandler.GinEngine.GET("/conversations/:igsid", ccapis.GetConversationById)
	apihandler.GinEngine.PUT("/conversations/:igsid", ccapis.UpdateConversation)

	apihandler.GinEngine.GET("/messages/:igsid", ccapis.GetMessages)
	apihandler.GinEngine.POST("/messages/:igsid", ccapis.SendMessage)

	apihandler.StartLambda()
}
