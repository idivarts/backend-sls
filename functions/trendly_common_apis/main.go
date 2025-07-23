package main

import (
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/trendlyapis"
	trendlyCollabs "github.com/idivarts/backend-sls/internal/trendlyapis/collaborations"
	apihandler "github.com/idivarts/backend-sls/pkg/api_handler"
)

func main() {
	commonV1 := apihandler.GinEngine.Group("/api/v1", middlewares.ValidateSessionMiddleware(), middlewares.TrendlyMiddleware("common"))

	commonV1.POST("/chat/auth", trendlyapis.ChatAuth)
	commonV1.POST("/chat/connect", trendlyapis.ChatConnect)
	commonV1.POST("/chat/channel", trendlyapis.ChatChannel)

	commonV1.POST("/contracts/:contractId", trendlyCollabs.StartContract)   // if called by influencer - ask, else start the contract
	commonV1.POST("/contracts/:contractId/end", trendlyCollabs.EndContract) // if called by influencer - ask, else end contract

	commonV1.DELETE("/users/deactivate", trendlyapis.DeativateUser)
	commonV1.DELETE("/users/delete", trendlyapis.DeleteUser)

	apihandler.StartLambda()
}
