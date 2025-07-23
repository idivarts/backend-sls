package main

import (
	"github.com/idivarts/backend-sls/internal/matchmaking"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/trendlyapis"
	trendlyCollabs "github.com/idivarts/backend-sls/internal/trendlyapis/collaborations"
	influencerv2 "github.com/idivarts/backend-sls/internal/trendlyapis/influencerV2"
	apihandler "github.com/idivarts/backend-sls/pkg/api_handler"
)

func main() {
	handleUserAPIs()
	handleManagerAPIs()

	commonV1 := apihandler.GinEngine.Group("/api/v1", middlewares.ValidateSessionMiddleware(), middlewares.TrendlyMiddleware("common"))

	commonV1.POST("/chat/auth", trendlyapis.ChatAuth)
	commonV1.POST("/chat/connect", trendlyapis.ChatConnect)
	commonV1.POST("/chat/channel", trendlyapis.ChatChannel)

	commonV1.POST("/contracts/:contractId", trendlyCollabs.StartContract)   // if called by influencer - ask, else start the contract
	commonV1.POST("/contracts/:contractId/end", trendlyCollabs.EndContract) // if called by influencer - ask, else end contract

	commonV1.DELETE("/users/deactivate", trendlyapis.DeativateUser)
	commonV1.DELETE("/users/delete", trendlyapis.DeleteUser)

	apihandler.StartLambda()
}
func handleManagerAPIs() {
	managerApisV1 := apihandler.GinEngine.Group("/api/v1", middlewares.ValidateSessionMiddleware(), middlewares.TrendlyMiddleware("managers"))

	managerApisV1.POST("/brands/members", trendlyapis.CreateBrandMember)

	managerApisV1.POST("/collaborations/:collabId/invitations/:userId", trendlyCollabs.SendInvitation)

	managerApisV1.POST("/collaborations/:collabId/applications/:userId/:action", trendlyCollabs.ApplicationAction) // accept|reject|revise

	// Managers Explore influencer api
	managerApisV1.GET("/influencers", matchmaking.GetInfluencers)
}

func handleUserAPIs() {
	userApisV1 := apihandler.GinEngine.Group("/api/v1", middlewares.ValidateSessionMiddleware(), middlewares.TrendlyMiddleware("users"))

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

	userApisV1.GET("/influencers/invite", influencerv2.GetInfluencerIDs)
	userApisV1.POST("/influencers/invite/:influencerId", influencerv2.InviteInfluencer)
	userApisV1.POST("/influencers/invite/:influencerId/accept", influencerv2.AcceptInfluencerInvite)
	userApisV1.POST("/influencers/invite/:influencerId/reject", influencerv2.RejectInfluencerInvite)
}
