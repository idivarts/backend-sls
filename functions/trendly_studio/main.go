package main

import (
	"github.com/idivarts/backend-sls/internal/trendlyapis/canva"
	"github.com/idivarts/backend-sls/internal/trendlyapis/media"
	"github.com/idivarts/backend-sls/internal/trendlyapis/studio"
	apihandler "github.com/idivarts/backend-sls/pkg/api_handler"
)

// trendly_studio hosts the Design Studio and the integrations it leans on:
// the studio and its scenes (/api/studio/*), ElevenLabs audio (/api/media/*)
// and the Canva Connect deep-edit bridge (/api/integrations/canva/*).
func main() {
	studio.RegisterRoutes(apihandler.GinEngine)
	media.RegisterRoutes(apihandler.GinEngine)
	canva.RegisterRoutes(apihandler.GinEngine)
	apihandler.StartLambda()
}
