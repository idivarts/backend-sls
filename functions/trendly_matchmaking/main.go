package main

import (
	"github.com/idivarts/backend-sls/internal/matchmaking"
	"github.com/idivarts/backend-sls/internal/middlewares"
	apihandler "github.com/idivarts/backend-sls/pkg/api_handler"
)

func main() {
	handleUserAPIs()
	handleManagerAPIs()

	apihandler.StartLambda()
}
func handleManagerAPIs() {
	managerApisV1 := apihandler.GinEngine.Group("/api/matchmaking", middlewares.ValidateSessionMiddleware(), middlewares.TrendlyMiddleware("managers"))

	managerApisV1.GET("/influencer-for-brand", matchmaking.GetInfluencers)
}

func handleUserAPIs() {
	userApisV1 := apihandler.GinEngine.Group("/api/search", middlewares.ValidateSessionMiddleware(), middlewares.TrendlyMiddleware("users"))

	userApisV1.GET("/influencer-for-influencer", matchmaking.GetInfluencerForInfluencer)
	userApisV1.GET("/collaborations", matchmaking.GetCollaborationIDs) // This api will be used to get the list of collaboration
}
