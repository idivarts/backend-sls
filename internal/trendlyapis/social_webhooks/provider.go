// Package socialwebhooks receives inbound social-platform webhooks (DMs and
// comments) and feeds them into the Trendly brand Inbox.
//
// It is platform-agnostic by design. Instagram and Facebook are implemented now
// (both Meta, sharing the same payload + X-Hub-Signature-256 scheme). Future
// platforms — LinkedIn, X/Twitter, YouTube — each add their own WebhookProvider
// implementation and register a route pair in the entrypoint
// (functions/trendly_social_webhooks/main.go).
//
// This package owns only the transport concerns: subscription handshake,
// signature verification, payload parsing and dispatch. The actual persistence
// lives in internal/trendlyapis/inbox (IngestMessaging / IngestComment), which
// resolves each event to a brand via socialAccountIndex.
package socialwebhooks

import "github.com/gin-gonic/gin"

// WebhookProvider is the per-platform contract for inbound social webhooks.
type WebhookProvider interface {
	// Verify handles the subscription handshake (GET). It echoes the platform's
	// challenge back when the verify token matches.
	Verify(c *gin.Context)

	// Receive handles an event delivery (POST): read the raw body, verify the
	// signature, parse the payload, and dispatch normalized events to the inbox.
	Receive(c *gin.Context)
}
