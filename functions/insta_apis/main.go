package main

import (
	"github.com/idivarts/backend-sls/internal/trendlyapis"
	apihandler "github.com/idivarts/backend-sls/pkg/api_handler"
)

func main() {
	apiV1 := apihandler.GinEngine.Group("/instagram")

	apiV1.GET("/", trendlyapis.InstagramRedirect)
	apiV1.GET("/auth/:redirect_type", trendlyapis.InstagramAuthRedirect)
	apiV1.POST("/auth", trendlyapis.InstagramAuth)
	apiV1.GET("/deauth", trendlyapis.InstagramDeAuth)
	apiV1.GET("/delete", trendlyapis.InstagramDelete)

	apihandler.StartLambda()
}
