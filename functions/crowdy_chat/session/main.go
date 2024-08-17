package main

import (
	crowdychat "github.com/TrendsHub/th-backend/internal/crowdy_chat"
	"github.com/TrendsHub/th-backend/internal/middlewares"
	apihandler "github.com/TrendsHub/th-backend/pkg/api_handler"
)

func main() {
	// sessionRoutes := apihandler.GinEngine.Group("", middlewares.ValidateSessionMiddleware())
	apihandler.GinEngine.Use(middlewares.ValidateSessionMiddleware())

	apihandler.GinEngine.GET("/organizations", crowdychat.GetOrganizations)
	apihandler.GinEngine.GET("/organizations/:orgId", crowdychat.GetOrganizationByID)
	apihandler.GinEngine.POST("/organizations", crowdychat.CreateOrganization)

	apihandler.GinEngine.PUT("/profile", crowdychat.UpdateProfile)

	apihandler.StartLambda()
}
