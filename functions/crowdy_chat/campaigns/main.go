package main

import (
	cc_campaigns "github.com/TrendsHub/th-backend/internal/crowdy_chat/campaigns"
	"github.com/TrendsHub/th-backend/internal/middlewares"
	apihandler "github.com/TrendsHub/th-backend/pkg/api_handler"
)

func main() {
	organizationRoutes := apihandler.GinEngine.Group("/campaigns", middlewares.ValidateSessionMiddleware(), middlewares.ValidateOrganizationMiddleware())

	organizationRoutes.GET("/", cc_campaigns.GetCampaigns)
	organizationRoutes.GET("/:id", cc_campaigns.GetCampaignByID)
	organizationRoutes.POST("/", cc_campaigns.CreateCampaign)
	organizationRoutes.PUT("/:id", cc_campaigns.UpdateCampaign)

	apihandler.StartLambda()
}
