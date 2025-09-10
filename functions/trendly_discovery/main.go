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
	brandAPIs.GET("/:brandId/influencers/:influencerId", trendlydiscovery.FetchInfluencer)
	brandAPIs.POST("/:brandId/influencers/:influencerId", trendlydiscovery.RequestConnection)

	// Creating a completely open route for image-relay
	apihandler.GinEngine.GET("/discovery/image-relay", trendlydiscovery.ImageRelay)
}
