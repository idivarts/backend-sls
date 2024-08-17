package main

import (
	crowdychat "github.com/TrendsHub/th-backend/internal/crowdy_chat"
	apihandler "github.com/TrendsHub/th-backend/pkg/api_handler"
)

func main() {
	apihandler.GinEngine.GET("/organizations", crowdychat.GetOrganizations)
	apihandler.GinEngine.GET("/organizations/:orgId", crowdychat.GetOrganizationByID)
	apihandler.GinEngine.POST("/organizations", crowdychat.CreateOrganization)

	apihandler.GinEngine.GET("/campaigns", crowdychat.GetCampaigns)
	apihandler.GinEngine.GET("/campaigns/:id", crowdychat.GetCampaignByID)
	apihandler.GinEngine.POST("/campaigns", crowdychat.CreateCampaign)
	apihandler.GinEngine.PUT("/campaigns/:id", crowdychat.UpdateCampaign)

	apihandler.StartLambda()
}
