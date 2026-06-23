package main

import (
	socialwebhooks "github.com/idivarts/backend-sls/internal/trendlyapis/social_webhooks"
	apihandler "github.com/idivarts/backend-sls/pkg/api_handler"
)

// trendly_social_webhooks receives inbound social-platform webhooks (DMs and
// comments) for the brand Inbox. Public callbacks — no session middleware;
// authenticity is established by the per-platform verify token (GET handshake)
// and the HMAC signature on each POST.
func main() {
	ig := socialwebhooks.NewInstagramProvider()
	fb := socialwebhooks.NewFacebookProvider()

	webhooks := apihandler.GinEngine.Group("/webhooks")

	// Instagram (DMs + comments/mentions).
	webhooks.GET("/instagram", ig.Verify)
	webhooks.POST("/instagram", ig.Receive)

	// Facebook (Messenger DMs + page feed comments).
	webhooks.GET("/facebook", fb.Verify)
	webhooks.POST("/facebook", fb.Receive)

	// Future social providers (LinkedIn / X / YouTube) register their route pair
	// here, each backed by its own WebhookProvider implementation.

	// Meta data-deletion callback (required for App Review).
	webhooks.POST("/data-deletion", socialwebhooks.DataDeletion)
	webhooks.GET("/data-deletion/status", socialwebhooks.DataDeletionStatus)

	apihandler.StartLambda()
}
