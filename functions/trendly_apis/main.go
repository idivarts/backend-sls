package main

import (
	"github.com/idivarts/backend-sls/internal/matchmaking"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/trendlyapis"
	trendlyCollabs "github.com/idivarts/backend-sls/internal/trendlyapis/collaborations"
	apihandler "github.com/idivarts/backend-sls/pkg/api_handler"
)

func main() {
	userApisV1 := apihandler.GinEngine.Group("/api/v1", middlewares.ValidateSessionMiddleware(), middlewares.TrendlyMiddleware("users"))
	managerApisV1 := apihandler.GinEngine.Group("/api/v1", middlewares.ValidateSessionMiddleware(), middlewares.TrendlyMiddleware("managers"))
	commonV1 := apihandler.GinEngine.Group("/api/v1", middlewares.ValidateSessionMiddleware(), middlewares.TrendlyMiddleware("common"))

	userApisV1.POST("/socials/facebook", trendlyapis.FacebookLogin)
	userApisV1.POST("/socials/instagram", trendlyapis.ConnectInstagram)
	userApisV1.POST("/socials/instagram/manual", trendlyapis.ConnectInstagramManual)

	// Calculate Insights
	userApisV1.POST("/socials/insights", trendlyapis.FetchInsights)

	// Get Social Medias
	userApisV1.GET("/socials/medias", trendlyapis.FetchMedias)

	commonV1.POST("/chat/auth", trendlyapis.ChatAuth)
	commonV1.POST("/chat/connect", trendlyapis.ChatConnect)
	commonV1.POST("/chat/channel", trendlyapis.ChatChannel)
	// userApisV1.POST("/chat/notification", trendlyapis.Notify)

	managerApisV1.POST("/brands/members", trendlyapis.CreateBrandMember)

	managerApisV1.POST("/collaborations/:collabId/invitations/:userId", trendlyCollabs.SendInvitation)
	userApisV1.POST("/collaborations/:collabId/applications/:userId", trendlyCollabs.SendApplication)
	userApisV1.PUT("/collaborations/:collabId/applications/:userId", trendlyCollabs.EditApplication)

	managerApisV1.POST("/collaborations/:collabId/applications/:userId/:action", trendlyCollabs.ApplicationAction) // accept|reject|revise

	commonV1.POST("/contracts/:contractId", trendlyCollabs.StartContract)   // if called by influencer - ask, else start the contract
	commonV1.POST("/contracts/:contractId/end", trendlyCollabs.EndContract) // if called by influencer - ask, else end contract
	userApisV1.POST("/contracts/:contractId/feedback", trendlyCollabs.GiveContractFeedback)

	commonV1.DELETE("/users/deactivate", trendlyapis.DeativateUser)
	commonV1.DELETE("/users/delete", trendlyapis.DeleteUser)

	// Managers Explore influencer api
	managerApisV1.GET("/influencers", matchmaking.GetInfluencers)

	apihandler.StartLambda()
}
