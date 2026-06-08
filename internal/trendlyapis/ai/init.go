package ai

import (
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
)

func RegisterRoutes(engine *gin.Engine) {
	g := engine.Group("/api/ai",
		middlewares.ValidateSessionMiddleware(),
		middlewares.TrendlyMiddleware("managers"),
	)

	g.POST("/conversations", CreateConversation)
	g.GET("/conversations", ListConversations)
	g.GET("/conversations/:conversationId", GetConversation)
	g.DELETE("/conversations/:conversationId", DeleteConversation)
	g.PATCH("/conversations/:conversationId", RenameConversation)
	g.POST("/conversations/:conversationId/message", HTTPMessage)

	// Onboarding seeding — one-shot setup for the /onboarding "what next" branch.
	g.POST("/onboarding/strategy-init", HTTPOnboardingStrategyInit)
	g.POST("/onboarding/calendar-init", HTTPOnboardingCalendarInit)

	g.POST("/quick-edit", HTTPQuickEdit)

	g.POST("/content/caption", HTTPCaption)
	g.POST("/content/hashtags", HTTPHashtags)

	g.GET("/models", ListModels)

	// Strategy → calendar conversion lives under its own prefix (documented
	// contract), same manager-session middleware.
	cs := engine.Group("/api/content-strategy",
		middlewares.ValidateSessionMiddleware(),
		middlewares.TrendlyMiddleware("managers"),
	)
	cs.POST("/:strategyId/push-to-calendar", HTTPPushToCalendar)
	cs.POST("/:strategyId/recheck-duration", HTTPRecheckDuration)
}
