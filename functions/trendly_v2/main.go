package main

import (
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/trendlyapis"
	"github.com/idivarts/backend-sls/internal/trendlyapis/analytics"
	"github.com/idivarts/backend-sls/internal/trendlyapis/inbox"
	"github.com/idivarts/backend-sls/internal/trendlyapis/publishing"
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

	// ── Organizations (top-level tenant above Brand) ──────────────────────────
	// Reads (list my orgs + org detail) go straight to Firestore from the app —
	// see contexts/organization-context.provider.tsx. Mutations stay here
	// because they need transactions and plan-cap / active-contract guards.
	managerApisV1.POST("/organizations", trendlyapis.CreateOrganization)
	managerApisV1.POST("/organizations/:id/brands", trendlyapis.AddBrandToOrganization)
	managerApisV1.DELETE("/organizations/:id", trendlyapis.DeleteOrganization)
	// Move a brand into an organization the caller owns (cap-enforced).
	managerApisV1.POST("/organizations/:id/brands/:brandId/transfer", trendlyapis.TransferBrand)
	// Soft-delete a brand (blocked while it has active contracts).
	managerApisV1.DELETE("/brands/:brandId", trendlyapis.DeleteBrand)

	// ── Brand member management (team assignment) ─────────────────────────────
	// Reads (list members) are served directly from Firestore by the apps.
	managerApisV1.PATCH("/brands/:brandId/members/:managerId", trendlyapis.UpdateBrandMember)
	managerApisV1.DELETE("/brands/:brandId/members/:managerId", trendlyapis.RemoveBrandMember)

	// ── Teams (brands/{brandId}/teams) ────────────────────────────────────────
	// Reads (list teams) are served directly from Firestore by the apps.
	managerApisV1.POST("/brands/:brandId/teams", trendlyapis.CreateTeam)
	managerApisV1.PATCH("/brands/:brandId/teams/:teamId", trendlyapis.UpdateTeam)
	managerApisV1.DELETE("/brands/:brandId/teams/:teamId", trendlyapis.DeleteTeam)

	// ── Brand social accounts (brands/{brandId}/socialAccounts) ───────────────
	managerApisV1.GET("/brands/:brandId/socials", social_connect.ListBrandSocials)
	managerApisV1.DELETE("/brands/:brandId/socials/:id", social_connect.DeleteBrandSocial)

	// ── Content publishing + scheduling (brands/{brandId}/contents) ───────────
	managerApisV1.POST("/brands/:brandId/contents/:contentId/publish", publishing.PublishNow)
	managerApisV1.POST("/brands/:brandId/contents/:contentId/schedule", publishing.SchedulePublish)
	managerApisV1.DELETE("/brands/:brandId/contents/:contentId/schedule", publishing.CancelSchedule)

	// ── Analytics / Reporting (unified Meta insights) ─────────────────────────
	managerApisV1.GET("/brands/:brandId/analytics/overview", analytics.GetBrandAnalyticsOverview)
	managerApisV1.GET("/brands/:brandId/analytics/accounts/:id", analytics.GetBrandAccountAnalytics)
	// Per-post basic analytics for a single published media (Content details page).
	managerApisV1.GET("/brands/:brandId/analytics/media/:mediaId", analytics.GetPostAnalytics)

	// ── Inbox (omni-channel DMs + comments across connected Meta accounts) ────
	managerApisV1.GET("/brands/:brandId/inbox", inbox.GetInbox)
	managerApisV1.POST("/brands/:brandId/inbox/sync", inbox.SyncInbox)
	managerApisV1.POST("/brands/:brandId/inbox/conversations/:id/reply", inbox.ReplyToConversation)
	managerApisV1.POST("/brands/:brandId/inbox/conversations/:id/hide", inbox.HideComment)
	managerApisV1.DELETE("/brands/:brandId/inbox/conversations/:id", inbox.DeleteConversation)
	managerApisV1.POST("/brands/:brandId/inbox/conversations/:id/read", inbox.ReadConversation)

	// Media tab — browse published posts/reels and their comments (on-demand
	// Graph reads). Comment actions are keyed by comment id (no stored conv).
	managerApisV1.GET("/brands/:brandId/inbox/media", inbox.GetMediaList)
	managerApisV1.GET("/brands/:brandId/inbox/media/:mediaId/comments", inbox.GetMediaComments)
	managerApisV1.POST("/brands/:brandId/inbox/comments/:commentId/reply", inbox.ReplyToMediaCommentHandler)
	managerApisV1.POST("/brands/:brandId/inbox/comments/:commentId/hide", inbox.HideMediaCommentHandler)
	managerApisV1.DELETE("/brands/:brandId/inbox/comments/:commentId", inbox.DeleteMediaCommentHandler)
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
