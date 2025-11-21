package main

import (
	"github.com/idivarts/backend-sls/internal/middlewares"
	trendlydiscovery "github.com/idivarts/backend-sls/internal/trendly_discovery"
	apihandler "github.com/idivarts/backend-sls/pkg/api_handler"
)

func main() {
	handleUserAPIs()

	apihandler.StartLambda()
}

func handleUserAPIs() {
	discoveryApi := apihandler.GinEngine.Group("/discovery", middlewares.TrendlyExtension())

	discoveryApi.POST("/extension", trendlydiscovery.AddProfile)
	discoveryApi.GET("/extension", trendlydiscovery.CheckUsername)

	brandAPIs := apihandler.GinEngine.Group("/discovery/brands", middlewares.ValidateSessionMiddleware(), middlewares.TrendlyMiddleware("managers"))

	brandAPIs.POST("/:brandId/influencers", trendlydiscovery.GetInfluencers)
	brandAPIs.POST("/:brandId/influencers/invite", trendlydiscovery.InviteInfluencerOnDiscover)
	brandAPIs.POST("/:brandId/collaborations/:collabId/influencers", trendlydiscovery.FetchInvitedInfluencers)

	brandAPIs.GET("/:brandId/influencers/:influencerId", trendlydiscovery.FetchInfluencer)
}
