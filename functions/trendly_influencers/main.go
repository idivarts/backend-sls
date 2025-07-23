package main

import (
	"github.com/idivarts/backend-sls/internal/middlewares"
	influencerv2 "github.com/idivarts/backend-sls/internal/trendlyapis/influencerV2"
	apihandler "github.com/idivarts/backend-sls/pkg/api_handler"
)

func main() {
	handleUserAPIs()

	apihandler.StartLambda()
}

func handleUserAPIs() {
	userApisV1 := apihandler.GinEngine.Group("/api/influencers", middlewares.ValidateSessionMiddleware(), middlewares.TrendlyMiddleware("users"))

	userApisV1.POST("/invite/:influencerId", influencerv2.InviteInfluencer)
	userApisV1.POST("/invite/:influencerId/accept", influencerv2.AcceptInfluencerInvite)
	userApisV1.POST("/invite/:influencerId/reject", influencerv2.RejectInfluencerInvite)
}
