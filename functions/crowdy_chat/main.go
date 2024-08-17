package main

import (
	crowdychat "github.com/TrendsHub/th-backend/internal/crowdy_chat"
	apihandler "github.com/TrendsHub/th-backend/pkg/api_handler"
)

func main() {
	apihandler.GinEngine.GET("/organizations", crowdychat.GetOrganizations)
	apihandler.GinEngine.GET("/organizations/:orgId", crowdychat.GetOrganizationByID)
	apihandler.GinEngine.POST("/organizations", crowdychat.CreateOrganization)

	apihandler.StartLambda()
}
