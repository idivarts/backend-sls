package main

import (
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/trendlyapis"
	"github.com/idivarts/backend-sls/internal/trendlyapis/social_connect"
	apihandler "github.com/idivarts/backend-sls/pkg/api_handler"
)

func main() {
	handleManagerAPIs()
	handleUserAPIs()

	commonV1 := apihandler.GinEngine.Group("/api/v2", middlewares.ValidateSessionMiddleware(), middlewares.TrendlyMiddleware("common"))

	commonV1.POST("/chat/auth", trendlyapis.ChatAuth)
	commonV1.POST("/chat/connect", trendlyapis.ChatConnect)

	commonV1.DELETE("/users/deactivate", trendlyapis.DeativateUser)
	commonV1.DELETE("/users/delete", trendlyapis.DeleteUser)

	apihandler.StartLambda()
}
func handleManagerAPIs() {
	managerApisV1 := apihandler.GinEngine.Group("/api/v2", middlewares.ValidateSessionMiddleware(), middlewares.TrendlyMiddleware("managers"))
	managerApisV1.POST("/brands/members", trendlyapis.CreateBrandMember)
	managerApisV1.POST("/brands/create", trendlyapis.CreateBrand)

	// ── Brand member management (role / teams / override toggles) ─────────────
	managerApisV1.GET("/brands/:brandId/members", trendlyapis.ListBrandMembers)
	managerApisV1.PATCH("/brands/:brandId/members/:managerId", trendlyapis.UpdateBrandMember)
	managerApisV1.DELETE("/brands/:brandId/members/:managerId", trendlyapis.RemoveBrandMember)

	// ── Teams (brands/{brandId}/teams) ────────────────────────────────────────
	managerApisV1.GET("/brands/:brandId/teams", trendlyapis.ListTeams)
	managerApisV1.POST("/brands/:brandId/teams", trendlyapis.CreateTeam)
	managerApisV1.PATCH("/brands/:brandId/teams/:teamId", trendlyapis.UpdateTeam)
	managerApisV1.DELETE("/brands/:brandId/teams/:teamId", trendlyapis.DeleteTeam)

	// ── Brand social accounts (brands/{brandId}/socialAccounts) ───────────────
	managerApisV1.GET("/brands/:brandId/socials", social_connect.ListBrandSocials)
	managerApisV1.DELETE("/brands/:brandId/socials/:id", social_connect.DeleteBrandSocial)
	managerApisV1.POST("/brands/:brandId/socials/:id/team", trendlyapis.AssignSocialTeam)
}

func handleUserAPIs() {
	userApisV1 := apihandler.GinEngine.Group("/api/v2", middlewares.ValidateSessionMiddleware(), middlewares.TrendlyMiddleware("users"))

	// Legacy social connect (pre-connect-portal flow — kept for backward compat)
	userApisV1.POST("/socials/facebook", trendlyapis.FacebookLogin)
	userApisV1.POST("/socials/instagram", trendlyapis.ConnectInstagram)
	userApisV1.POST("/socials/instagram/manual", trendlyapis.ConnectInstagramManual)

	// Calculate Insights
	userApisV1.POST("/socials/insights", trendlyapis.FetchInsights)

	// Get Social Medias
	userApisV1.GET("/socials/medias", trendlyapis.FetchMedias)

	// ── Social V2 (connect-portal OAuth flow) ─────────────────────────────────
	userApisV1.GET("/socials/v2", social_connect.ListSocials)
	userApisV1.DELETE("/socials/v2/:id", social_connect.DeleteSocial)
}
