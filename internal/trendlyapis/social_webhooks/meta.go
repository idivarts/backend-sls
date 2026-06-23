package socialwebhooks

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/internal/trendlyapis/inbox"
	instainterfaces "github.com/idivarts/backend-sls/pkg/interfaces/instaInterfaces"
)

// MetaProvider implements WebhookProvider for the Meta platforms — Instagram and
// Facebook — which share an identical webhook payload (entry[].messaging for DMs,
// entry[].changes for comments/feed) and the same X-Hub-Signature-256 signing
// scheme. Instagram and Facebook are registered as two separate instances so
// each owns its own callback route and verify token (and can diverge later),
// while sharing the parsing/dispatch internals below.
type MetaProvider struct {
	// platform labels the provider (logs only); the inbox resolves the actual
	// display channel per event from socialAccountIndex.
	platform trendlymodels.Platform
	// name is the route/log identifier, e.g. "instagram" | "facebook".
	name string
	// verifyTokenEnv is the platform-specific verify-token env var.
	verifyTokenEnv string
}

// NewInstagramProvider returns the Meta provider bound to the Instagram channel.
func NewInstagramProvider() *MetaProvider {
	return &MetaProvider{
		platform:       trendlymodels.PlatformInstagram,
		name:           "instagram",
		verifyTokenEnv: "WEBHOOK_VERIFY_TOKEN_INSTAGRAM",
	}
}

// NewFacebookProvider returns the Meta provider bound to the Facebook channel.
func NewFacebookProvider() *MetaProvider {
	return &MetaProvider{
		platform:       trendlymodels.PlatformFacebook,
		name:           "facebook",
		verifyTokenEnv: "WEBHOOK_VERIFY_TOKEN_FACEBOOK",
	}
}

// verifyToken resolves the platform's subscription verify token: the
// platform-specific env var first, then a shared WEBHOOK_VERIFY_TOKEN fallback,
// then the historical default so existing dashboard subscriptions keep working.
func (p *MetaProvider) verifyToken() string {
	if t := os.Getenv(p.verifyTokenEnv); t != "" {
		return t
	}
	if t := os.Getenv("WEBHOOK_VERIFY_TOKEN"); t != "" {
		return t
	}
	return "mytoken"
}

// webhookSubscription is the GET handshake query Meta sends on "Verify and Save".
type webhookSubscription struct {
	Mode        string `form:"hub.mode" binding:"required"`
	VerifyToken string `form:"hub.verify_token" binding:"required"`
	Challenge   string `form:"hub.challenge" binding:"required"`
}

// Verify handles the subscription handshake. A matching token echoes the
// challenge back verbatim (plain text), which is what Meta expects.
func (p *MetaProvider) Verify(c *gin.Context) {
	var req webhookSubscription
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Mode != "subscribe" || req.VerifyToken != p.verifyToken() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request parameters"})
		return
	}
	c.String(http.StatusOK, "%s", req.Challenge)
}

// Receive ingests a Meta event delivery. The raw body is read first (needed for
// HMAC verification — binding would consume the stream and re-serialization
// would change the bytes), the signature is checked, then DMs and comments are
// dispatched to the inbox. Always returns 200 on a well-formed body so Meta does
// not retry events that were intentionally ignored (e.g. non-brand accounts).
func (p *MetaProvider) Receive(c *gin.Context) {
	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
		return
	}

	if !verifyMetaSignature(rawBody, c.GetHeader("X-Hub-Signature-256")) {
		log.Printf("social_webhooks/%s: signature verification failed", p.name)
		if strictSignature() {
			c.JSON(http.StatusForbidden, gin.H{"error": "invalid signature"})
			return
		}
	}

	var msg instainterfaces.IMessageWebhook
	if err := json.Unmarshal(rawBody, &msg); err != nil {
		log.Printf("social_webhooks/%s: bad payload: %v", p.name, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	p.dispatch(&msg)
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

// dispatch routes each entry's DMs and comments to the inbox ingestion. Events
// for accounts not connected by a brand are no-ops inside the inbox package.
func (p *MetaProvider) dispatch(msg *instainterfaces.IMessageWebhook) {
	for i := range msg.Entry {
		sourceID := msg.Entry[i].ID

		// Direct messages (entry.messaging[]).
		for j := range msg.Entry[i].Messaging {
			inbox.IngestMessaging(sourceID, &msg.Entry[i].Messaging[j])
		}

		// Comments / mentions / feed (entry.changes[]).
		for k := range msg.Entry[i].Changes {
			inbox.IngestComment(sourceID, &msg.Entry[i].Changes[k])
		}
	}
}
