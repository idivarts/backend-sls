package main

import (
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/trendlyapis"
	trendlyCollabs "github.com/idivarts/backend-sls/internal/trendlyapis/collaborations"
	apihandler "github.com/idivarts/backend-sls/pkg/api_handler"
)

func main() {
	handleManagerAPIs()
	apihandler.StartLambda()
}
func handleManagerAPIs() {
	managerApisV1 := apihandler.GinEngine.Group("/api/managers", middlewares.ValidateSessionMiddleware(), middlewares.TrendlyMiddleware("managers"))

	managerApisV1.POST("/brands/members", trendlyapis.CreateBrandMember)

	managerApisV1.POST("/collaborations/:collabId/invitations/:userId", trendlyCollabs.SendInvitation)

	managerApisV1.POST("/collaborations/:collabId/applications/:userId/:action", trendlyCollabs.ApplicationAction) // accept|reject|revise
}
