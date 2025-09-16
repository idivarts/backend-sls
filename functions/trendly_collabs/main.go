package main

import (
	"github.com/idivarts/backend-sls/internal/middlewares"
	trendlyCollabs "github.com/idivarts/backend-sls/internal/trendlyapis/collaborations"
	apihandler "github.com/idivarts/backend-sls/pkg/api_handler"
)

func main() {
	handleUserAPIs()
	handleManagerAPIs()

	commonV1 := apihandler.GinEngine.Group("/api/collabs", middlewares.ValidateSessionMiddleware(), middlewares.TrendlyMiddleware("common"))

	commonV1.POST("/contracts/:contractId", trendlyCollabs.StartContract)   // if called by influencer - ask, else start the contract
	commonV1.POST("/contracts/:contractId/end", trendlyCollabs.EndContract) // if called by influencer - ask, else end contract

	apihandler.StartLambda()
}
func handleManagerAPIs() {
	managerApisV1 := apihandler.GinEngine.Group("/api/collabs", middlewares.ValidateSessionMiddleware(), middlewares.TrendlyMiddleware("managers"))

	managerApisV1.POST("/influencers/:influencerId/unlock", trendlyCollabs.InfluencerUnlocked)
	managerApisV1.POST("/influencers/:influencerId/message", trendlyCollabs.SendMessage)

	managerApisV1.POST("/collaborations/:collabId", trendlyCollabs.PostCollaboration)

	managerApisV1.POST("/collaborations/:collabId/invitations/:userId", trendlyCollabs.SendInvitation)

	managerApisV1.POST("/collaborations/:collabId/applications/:userId/:action", trendlyCollabs.ApplicationAction) // accept|reject|revise

}

func handleUserAPIs() {
	userApisV1 := apihandler.GinEngine.Group("/api/collabs", middlewares.ValidateSessionMiddleware(), middlewares.TrendlyMiddleware("users"))

	userApisV1.POST("/collaborations/:collabId/applications/:userId", trendlyCollabs.SendApplication)
	userApisV1.PUT("/collaborations/:collabId/applications/:userId", trendlyCollabs.EditApplication)

	userApisV1.POST("/contracts/:contractId/feedback", trendlyCollabs.GiveContractFeedback)
}
