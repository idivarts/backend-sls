package main

import (
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/trendlyapis"
	apihandler "github.com/idivarts/backend-sls/pkg/api_handler"
)

func main() {
	handleManagerAPIs()
	handleUserAPIs()

	commonV1 := apihandler.GinEngine.Group("/api/v2", middlewares.ValidateSessionMiddleware(), middlewares.TrendlyMiddleware("common"))

	commonV1.POST("/chat/auth", trendlyapis.ChatAuth)
	commonV1.POST("/chat/connect", trendlyapis.ChatConnect)
	commonV1.POST("/chat/channel", trendlyapis.ChatChannel)

	commonV1.DELETE("/users/deactivate", trendlyapis.DeativateUser)
	commonV1.DELETE("/users/delete", trendlyapis.DeleteUser)

	apihandler.StartLambda()
}
func handleManagerAPIs() {
	managerApisV1 := apihandler.GinEngine.Group("/api/v2", middlewares.ValidateSessionMiddleware(), middlewares.TrendlyMiddleware("managers"))
	managerApisV1.POST("/brands/members", trendlyapis.CreateBrandMember)
	managerApisV1.POST("/brands/create", trendlyapis.CreateBrand)
}

func handleUserAPIs() {
	userApisV1 := apihandler.GinEngine.Group("/api/v2", middlewares.ValidateSessionMiddleware(), middlewares.TrendlyMiddleware("users"))

	userApisV1.POST("/socials/facebook", trendlyapis.FacebookLogin)
	userApisV1.POST("/socials/instagram", trendlyapis.ConnectInstagram)
	userApisV1.POST("/socials/instagram/manual", trendlyapis.ConnectInstagramManual)

	// Calculate Insights
	userApisV1.POST("/socials/insights", trendlyapis.FetchInsights)

	// Get Social Medias
	userApisV1.GET("/socials/medias", trendlyapis.FetchMedias)
}
