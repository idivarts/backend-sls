package main

import (
	"github.com/idivarts/backend-sls/internal/trendlyapis/canva"
	"github.com/idivarts/backend-sls/internal/trendlyapis/media"
	apihandler "github.com/idivarts/backend-sls/pkg/api_handler"
)

// trendly_studio hosts the AI-Studio integration endpoints: ElevenLabs audio
// (/api/media/*) and the Canva Connect deep-edit bridge
// (/api/integrations/canva/*).
func main() {
	media.RegisterRoutes(apihandler.GinEngine)
	canva.RegisterRoutes(apihandler.GinEngine)
	apihandler.StartLambda()
}
