package main

import (
	"github.com/idivarts/backend-sls/internal/trendlyapis/ai"
	apihandler "github.com/idivarts/backend-sls/pkg/api_handler"
)

func main() {
	ai.RegisterRoutes(apihandler.GinEngine)
	apihandler.StartLambda()
}
