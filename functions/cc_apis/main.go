package main

import (
	"github.com/TrendsHub/th-backend/internal/ccapis"
	"github.com/TrendsHub/th-backend/internal/middlewares"
	apihandler "github.com/TrendsHub/th-backend/pkg/api_handler"
)

func main() {
	apiV1 := apihandler.GinEngine.Group("/api/v1", middlewares.ValidateSessionMiddleware(), middlewares.ValidateOrganizationMiddleware())

	apiV1.POST("/sources/facebook", ccapis.FacebookLogin)

	apiV1.POST("/sources/:pageId/webhook", ccapis.PageWebhook)
	apiV1.POST("/sources/:pageId/sync", ccapis.PageSync)

	apiV1.PUT("/conversations/:leadId/stop", ccapis.StopConversation)

	apiV1.GET("/messages/:leadId", ccapis.GetMessages)
	apiV1.POST("/messages/:leadId", ccapis.SendMessage)

	apihandler.StartLambda()
}
