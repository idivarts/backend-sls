package main

import (
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/trendlyapis"
	apihandler "github.com/idivarts/backend-sls/pkg/api_handler"
)

func main() {
	apiV1 := apihandler.GinEngine.Group("/api/v1", middlewares.ValidateSessionMiddleware())

	apiV1.POST("/socials/facebook", trendlyapis.FacebookLogin)
	apiV1.POST("/socials/instagram", trendlyapis.InstagramAuth)

	apihandler.StartLambda()
}
