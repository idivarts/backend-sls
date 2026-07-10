// Package canva hosts the Canva Connect deep-edit bridge endpoints
// (/api/integrations/canva/*): OAuth connect/callback, design create/import,
// export, and return-navigation verification. Connecting is a plan entitlement
// (CanvaBridge, Pro+); it costs no AI tokens so it is not wallet-metered.
package canva

import (
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
)

// RegisterRoutes wires /api/integrations/canva. The callback is PUBLIC (Canva
// redirects to it without a Firebase session and correlates via `state`);
// everything else requires a manager session.
func RegisterRoutes(engine *gin.Engine) {
	// Public callback — no session.
	engine.GET("/api/integrations/canva/callback", Callback)

	g := engine.Group("/api/integrations/canva",
		middlewares.ValidateSessionMiddleware(),
		middlewares.TrendlyMiddleware("managers"),
	)
	g.GET("/status", Status)
	g.GET("/connect", ConnectStart)
	g.DELETE("/connect", Disconnect)
	g.GET("/designs", ListDesigns)
	g.POST("/brands/:brandId/contents/:contentId/design", CreateDesignFromContent)
	g.POST("/brands/:brandId/contents/:contentId/export", CreateExport)
	g.GET("/exports/:jobId", GetExport)
	g.POST("/brands/:brandId/contents/:contentId/return", HandleReturn)
}
