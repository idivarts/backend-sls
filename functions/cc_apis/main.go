package main

import (
	"github.com/TrendsHub/th-backend/internal/ccapis"
	apihandler "github.com/TrendsHub/th-backend/pkg/api_handler"
)

func main() {
	apiV1 := apihandler.GinEngine.Group("/api/v1")

	apiV1.POST("/sources/facebook", ccapis.FacebookLogin)

	// All newly changed apis below
	apiV1.POST("/sources/:pageId/webhook", ccapis.PageWebhook)
	apiV1.POST("/sources/:pageId/sync", ccapis.PageSync)

	apiV1.GET("/conversations", ccapis.GetConversations)

	apiV1.GET("/conversations/:igsid", ccapis.GetConversationById)
	apiV1.PUT("/conversations/:igsid", ccapis.UpdateConversation)

	apiV1.GET("/messages/:igsid", ccapis.GetMessages)
	apiV1.POST("/messages/:igsid", ccapis.SendMessage)

	apihandler.StartLambda()
}
