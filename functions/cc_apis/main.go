package main

import (
	"github.com/TrendsHub/th-backend/internal/ccapis"
	"github.com/TrendsHub/th-backend/internal/middlewares"
	apihandler "github.com/TrendsHub/th-backend/pkg/api_handler"
)

func main() {
	apiV1 := apihandler.GinEngine.Group("/api/v1", middlewares.ValidateSessionMiddleware(), middlewares.ValidateOrganizationMiddleware())

	apiV1.POST("/sources/facebook", ccapis.FacebookLogin)
	apiV1.POST("/sources/facebook/:sourceId/webhook", ccapis.PageWebhook)
	apiV1.POST("/sources/facebook/:sourceId/leads", ccapis.SourceSyncLeads) // We would use this api to create all the leads and fetch there profile

	apiV1.POST("/campaigns/:campaignId", ccapis.GetMessages)           //Initiates the campaigns by creating Assistant
	apiV1.POST("/campaigns/:campaignId/sync", ccapis.SourceSyncLeads)  //API to sync either all the connected sources, or specific sources or specific conversations
	apiV1.POST("/campaigns/:campaignId/sources", ccapis.GetMessages)   // Create API to attach source
	apiV1.DELETE("/campaigns/:campaignId/sources", ccapis.GetMessages) // Creatae API to Delete the attached source and its conversations

	apiV1.PUT("/conversations/:conversationId", ccapis.StopConversation) // Make changes in the api to stop tracking the conversation
	apiV1.GET("/conversations/:conversationId/messages", ccapis.GetMessages)
	apiV1.POST("/conversations/:conversationId/messages", ccapis.SendMessage)

	apihandler.StartLambda()
}
