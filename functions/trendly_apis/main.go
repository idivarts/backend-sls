package main

import (
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/trendlyapis"
	trendlyCollabs "github.com/idivarts/backend-sls/internal/trendlyapis/collaborations"
	apihandler "github.com/idivarts/backend-sls/pkg/api_handler"
)

func main() {
	apiV1 := apihandler.GinEngine.Group("/api/v1", middlewares.ValidateSessionMiddleware(), middlewares.TrendlyMiddleware())

	apiV1.POST("/socials/facebook", trendlyapis.FacebookLogin)
	apiV1.POST("/socials/instagram", trendlyapis.ConnectInstagram)
	apiV1.POST("/socials/instagram/manual", trendlyapis.ConnectInstagramManual)

	// Calculate Insights
	apiV1.POST("/socials/insights", trendlyapis.FetchInsights)

	// Get Social Medias
	apiV1.GET("/socials/medias", trendlyapis.FetchMedias)

	apiV1.POST("/chat/auth", trendlyapis.ChatAuth)
	apiV1.POST("/chat/connect", trendlyapis.ChatConnect)
	apiV1.POST("/chat/channel", trendlyapis.ChatChannel)
	apiV1.POST("/chat/notification", trendlyapis.Notify)

	apiV1.POST("/brands/members", trendlyapis.CreateBrandMember)

	apiV1.POST("/collaborations/:collabId/invitations/:userId", trendlyCollabs.SendInvitation)
	apiV1.POST("/collaborations/:collabId/applications/:userId", trendlyCollabs.SendApplication)
	apiV1.PUT("/collaborations/:collabId/applications/:userId", trendlyCollabs.EditApplication)

	apiV1.POST("/collaborations/:collabId/applications/:userId/:action", trendlyCollabs.ApplicationAction) // accept|reject|revise

	apiV1.POST("/contracts/:contractId", trendlyCollabs.StartContract)   // if called by influencer - ask, else start the contract
	apiV1.POST("/contracts/:contractId/end", trendlyCollabs.EndContract) // if called by influencer - ask, else end contract
	apiV1.POST("/contracts/:contractId/feedback", trendlyCollabs.GiveContractFeedback)

	apiV1.DELETE("/users/deactivate", trendlyapis.DeativateUser)
	apiV1.DELETE("/users/delete", trendlyapis.DeleteUser)

	apihandler.StartLambda()
}
