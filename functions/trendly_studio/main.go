package main

import (
	"github.com/idivarts/backend-sls/internal/trendlyapis/canva"
	"github.com/idivarts/backend-sls/internal/trendlyapis/design"
	"github.com/idivarts/backend-sls/internal/trendlyapis/media"
	"github.com/idivarts/backend-sls/internal/trendlyapis/studio"
	apihandler "github.com/idivarts/backend-sls/pkg/api_handler"
)

// trendly_studio hosts the Design Studio and the integrations it leans on:
// the studio and its scenes (/api/studio/*), Design Studio v3 and its designs
// (/api/design/*), ElevenLabs audio (/api/media/*) and the Canva Connect
// deep-edit bridge (/api/integrations/canva/*).
//
// v2 and v3 are registered side by side on purpose. They are different route
// trees over different collections — /api/studio is content-scoped, /api/design
// is not — and both stay live until Phase 6 retires v2.
func main() {
	studio.RegisterRoutes(apihandler.GinEngine)
	design.RegisterRoutes(apihandler.GinEngine)
	media.RegisterRoutes(apihandler.GinEngine)
	canva.RegisterRoutes(apihandler.GinEngine)
	apihandler.StartLambda()
}
