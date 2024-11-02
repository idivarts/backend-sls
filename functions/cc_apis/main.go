package main

import (
	campaignsapi "github.com/idivarts/backend-sls/internal/ccapis/campaigns"
	conversationsapi "github.com/idivarts/backend-sls/internal/ccapis/campaigns/conversations"
	finetune "github.com/idivarts/backend-sls/internal/ccapis/campaigns/fineTune"
	sourcesapi "github.com/idivarts/backend-sls/internal/ccapis/sources"
	"github.com/idivarts/backend-sls/internal/middlewares"
	apihandler "github.com/idivarts/backend-sls/pkg/api_handler"
)

func main() {
	apiV1 := apihandler.GinEngine.Group("/api/v1", middlewares.ValidateSessionMiddleware(), middlewares.ValidateOrganizationMiddleware())

	apiV1.POST("/sources/facebook", sourcesapi.FacebookLogin)
	apiV1.POST("/sources/facebook/:sourceId/webhook", sourcesapi.SourceWebhookAction)
	apiV1.POST("/sources/facebook/:sourceId/leads", sourcesapi.SourceSyncLeads) // We would use this api to create all the leads and fetch there profile

	apiV1.POST("/campaigns/:campaignId", campaignsapi.CreateOrUpdateCampaign) //Initiates the campaigns by creating Assistant

	apiV1.POST("/campaigns/:campaignId/sources", campaignsapi.ConnectSourcesWithCampaign)      //This api will be used ot connect sources
	apiV1.DELETE("/campaigns/:campaignId/sources", campaignsapi.DisconnectSourcesFromCampaign) //This api will disconnect a source from the campaign

	apiV1.POST("/campaigns/:campaignId/dryrun", finetune.Create)                 //TODO: This api will help in creating a test thread
	apiV1.GET("/campaigns/:campaignId/dryrun/:threadId", finetune.GetThread)     //TODO: This api will get all messages from the thread
	apiV1.POST("/campaigns/:campaignId/dryrun/:threadId", finetune.PostToThread) //TODO: This api will post a new message to the thread

	apiV1.PUT("/campaigns/:campaignId/conversations/:conversationId", conversationsapi.UpdateConversation)      // Make changes in the api to stop tracking the conversation
	apiV1.POST("/campaigns/:campaignId/conversations/:conversationId/sync", conversationsapi.SyncConversations) //API to sync a specific conversation
	apiV1.GET("/campaigns/:campaignId/conversations/:conversationId/messages", conversationsapi.GetMessages)
	apiV1.POST("/campaigns/:campaignId/conversations/:conversationId/messages", conversationsapi.SendMessage)

	apihandler.StartLambda()
}
