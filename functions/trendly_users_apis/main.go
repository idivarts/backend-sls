package main

import (
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/trendlyapis"
	trendlyCollabs "github.com/idivarts/backend-sls/internal/trendlyapis/collaborations"
	influencerv2 "github.com/idivarts/backend-sls/internal/trendlyapis/influencerV2"
	apihandler "github.com/idivarts/backend-sls/pkg/api_handler"
)

func main() {
	handleUserAPIs()
	apihandler.StartLambda()
}

func handleUserAPIs() {
	userApisV1 := apihandler.GinEngine.Group("/api/users", middlewares.ValidateSessionMiddleware(), middlewares.TrendlyMiddleware("users"))

	userApisV1.POST("/socials/facebook", trendlyapis.FacebookLogin)
	userApisV1.POST("/socials/instagram", trendlyapis.ConnectInstagram)
	userApisV1.POST("/socials/instagram/manual", trendlyapis.ConnectInstagramManual)

	// Calculate Insights
	userApisV1.POST("/socials/insights", trendlyapis.FetchInsights)

	// Get Social Medias
	userApisV1.GET("/socials/medias", trendlyapis.FetchMedias)

	userApisV1.GET("/collaborations", influencerv2.GetCollaborationIDs) // This api will be used to get the list of collaboration
	userApisV1.POST("/collaborations/:collabId/applications/:userId", trendlyCollabs.SendApplication)
	userApisV1.PUT("/collaborations/:collabId/applications/:userId", trendlyCollabs.EditApplication)

	userApisV1.POST("/contracts/:contractId/feedback", trendlyCollabs.GiveContractFeedback)
}
