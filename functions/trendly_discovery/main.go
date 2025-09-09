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
	userApisV1 := apihandler.GinEngine.Group("/discovery", middlewares.TrendlyExtension())

	userApisV1.POST("/extension", trendlydiscovery.AddProfile)
	userApisV1.GET("/extension", trendlydiscovery.CheckUsername)

	brandAPIs := apihandler.GinEngine.Group("/discovery/brands", middlewares.ValidateSessionMiddleware(), middlewares.TrendlyMiddleware("managers"))

	brandAPIs.POST("/:brandId/influencers", trendlydiscovery.GetInfluencers)
	brandAPIs.POST("/:brandId/influencers/:influencerId", trendlydiscovery.FetchInfluencer)
}
