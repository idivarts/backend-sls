package main

import (
	crowdychat "github.com/TrendsHub/th-backend/internal/crowdy_chat"
	cc_campaigns "github.com/TrendsHub/th-backend/internal/crowdy_chat/campaigns"
	"github.com/TrendsHub/th-backend/internal/middlewares"
	apihandler "github.com/TrendsHub/th-backend/pkg/api_handler"
)

func main() {
	sessionRoutes := apihandler.GinEngine.Group("", middlewares.ValidateSessionMiddleware())

	sessionRoutes.GET("/organizations", crowdychat.GetOrganizations)
	sessionRoutes.GET("/organizations/:orgId", crowdychat.GetOrganizationByID)
	sessionRoutes.POST("/organizations", crowdychat.CreateOrganization)

	sessionRoutes.PUT("/profile", crowdychat.UpdateProfile)

	organizationRoutes := sessionRoutes.Group("/main", middlewares.ValidateOrganizationMiddleware())

	organizationRoutes.GET("/campaigns", cc_campaigns.GetCampaigns)
	organizationRoutes.GET("/campaigns/:id", cc_campaigns.GetCampaignByID)
	organizationRoutes.POST("/campaigns", cc_campaigns.CreateCampaign)
	organizationRoutes.PUT("/campaigns/:id", cc_campaigns.UpdateCampaign)

	apihandler.StartLambda()
}
