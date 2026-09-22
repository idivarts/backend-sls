// Package media hosts the backend-owned audio generation endpoints (ElevenLabs
// music + voiceovers) used by the content composer. The ElevenLabs key stays
// server-side; every generation is metered against the org token wallet.
package media

import (
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
)

// RegisterRoutes wires the /api/media routes onto the engine. All routes require
// a manager (brand) session.
func RegisterRoutes(engine *gin.Engine) {
	g := engine.Group("/api/media",
		middlewares.ValidateSessionMiddleware(),
		middlewares.TrendlyMiddleware("managers"),
	)

	// Voice catalogue for the voiceover picker (includes preview_url per voice).
	g.GET("/voices", ListVoices)
	// Curated music catalog for the soundtrack browser (search + preview + select).
	g.GET("/music/library", ListMusicLibrary)

	// Per-brand generation + library.
	g.POST("/brands/:brandId/audio/music", GenerateMusic)
	g.POST("/brands/:brandId/audio/voiceover", GenerateVoiceover)
	g.GET("/brands/:brandId/audio", ListGeneratedAudio)
	// Deterministic design edits (text edit + revert) happen frontend-side now:
	// the designs subcollection is client-writable, so the app writes new HTML
	// revisions + captured renders directly (no backend round-trip).
}
