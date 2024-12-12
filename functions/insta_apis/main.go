package main

import (
	"github.com/idivarts/backend-sls/internal/trendlyapis"
	apihandler "github.com/idivarts/backend-sls/pkg/api_handler"
)

func main() {
	apiV1 := apihandler.GinEngine.Group("/instagram")

	apiV1.POST("/", trendlyapis.InstagramRedirect)
	apiV1.POST("/auth", trendlyapis.InstagramAuth)
	apiV1.POST("/deauth", trendlyapis.InstagramAuth)
	apiV1.POST("/delete", trendlyapis.InstagramAuth)

	apihandler.StartLambda()
}
