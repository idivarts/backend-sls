package main

import (
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	trendlyCollabs "github.com/idivarts/backend-sls/internal/trendlyapis/collaborations"
	apihandler "github.com/idivarts/backend-sls/pkg/api_handler"
)

func main() {
	handler := apihandler.GinEngine.Group("/api/collabs", middlewares.ValidateSessionMiddleware())
	handleUserAPIs(handler)
	handleManagerAPIs(handler)

	apihandler.StartLambda()
}
func handleManagerAPIs(handler *gin.RouterGroup) {
	managerApisV1 := handler.Group("/", middlewares.TrendlyMiddleware("managers"))

	managerApisV1.POST("/influencers/:influencerId/unlock", trendlyCollabs.InfluencerUnlocked)
	managerApisV1.POST("/influencers/:influencerId/message", trendlyCollabs.SendMessage)

	managerApisV1.POST("/collaborations", trendlyCollabs.CreateCollaborationWithPrompt)
	managerApisV1.POST("/collaborations/:collabId", trendlyCollabs.PostCollaboration)

	managerApisV1.POST("/collaborations/:collabId/invitations/:userId", trendlyCollabs.SendInvitation)

	managerApisV1.POST("/collaborations/:collabId/applications/:userId/:action", trendlyCollabs.ApplicationAction) // accept|reject|revise

	managerApisV1.POST("/contracts/:contractId/brand-feedback", trendlyCollabs.BrandFeedback)
}

func handleUserAPIs(handler *gin.RouterGroup) {
	userApisV1 := handler.Group("/", middlewares.TrendlyMiddleware("users"))

	userApisV1.POST("/contracts/:contractId", trendlyCollabs.RequestToStartContract)

	userApisV1.POST("/collaborations/:collabId/applications/:userId", trendlyCollabs.SendApplication) // Send Notification for new application
	userApisV1.PUT("/collaborations/:collabId/applications/:userId", trendlyCollabs.EditApplication)  // Send Notification for updated application

	userApisV1.POST("/contracts/:contractId/user-feedback", trendlyCollabs.UserFeedback)
}
