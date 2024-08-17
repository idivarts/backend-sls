package main

import (
	crowdychat "github.com/TrendsHub/th-backend/internal/crowdy_chat"
	"github.com/TrendsHub/th-backend/internal/middlewares"
	apihandler "github.com/TrendsHub/th-backend/pkg/api_handler"
)

func main() {
	sessionRoutes := apihandler.GinEngine.Group("", middlewares.ValidateSessionMiddleware())

	sessionRoutes.GET("/organizations", crowdychat.GetOrganizations)
	sessionRoutes.GET("/organizations/:orgId", crowdychat.GetOrganizationByID)
	sessionRoutes.POST("/organizations", crowdychat.CreateOrganization)

	sessionRoutes.PUT("/profile", crowdychat.UpdateProfile)

	apihandler.StartLambda()
}
