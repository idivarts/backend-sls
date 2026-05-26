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

	g.POST("/quick-edit", HTTPQuickEdit)

	g.POST("/content/caption", HTTPCaption)
	g.POST("/content/hashtags", HTTPHashtags)

	g.GET("/models", ListModels)
}
