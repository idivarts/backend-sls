package main

import (
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/trendlyapis"
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

	apihandler.StartLambda()
}
