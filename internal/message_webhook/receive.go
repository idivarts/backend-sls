package messagewebhook

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	mwh_handler "github.com/idivarts/backend-sls/internal/message_webhook/handler"
	"github.com/idivarts/backend-sls/internal/trendlyapis/inbox"
	instainterfaces "github.com/idivarts/backend-sls/pkg/interfaces/instaInterfaces"
)

func Receive(c *gin.Context) {
	// Read the raw body first — needed for HMAC signature verification (binding
	// would consume the stream and re-serialization would change the bytes).
	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
		return
	}

	// Verify X-Hub-Signature-256. In strict mode (WEBHOOK_STRICT_SIGNATURE=true)
	// a bad signature is rejected; otherwise we log and continue so the existing
	// chatbot flow is never broken by a config mismatch. Flip strict on once the
	// app secrets are confirmed in the environment.
	sig := c.GetHeader("X-Hub-Signature-256")
	if !verifyWebhookSignature(rawBody, sig) {
		log.Printf("message_webhook: signature verification failed (sig=%q)", sig)
		if os.Getenv("WEBHOOK_STRICT_SIGNATURE") == "true" {
			c.JSON(http.StatusForbidden, gin.H{"error": "invalid signature"})
			return
		}
	}

	var message instainterfaces.IMessageWebhook
	if err := json.Unmarshal(rawBody, &message); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		log.Println(err.Error())
		return
	}

	for i := 0; i < len(message.Entry); i++ {
		sourceId := message.Entry[i].ID

		// ── Direct messages ──────────────────────────────────────────────────
		for j := 0; j < len(message.Entry[i].Messaging); j++ {
			entry := &message.Entry[i].Messaging[j]

			// Inbox ingestion (brand-connected accounts). No-op for others.
			inbox.IngestMessaging(sourceId, entry)

			// Legacy chatbot handler (unchanged).
			if instainterfaces.CalcualateMessageType(entry) == instainterfaces.MessageTypeMessage {
				if herr := (mwh_handler.IGMessagehandler{
					LeadID:   entry.Sender.ID,
					Message:  entry.Message,
					SourceID: sourceId,
					Entry:    entry,
				}).HandleMessage(); herr != nil {
					log.Println(herr.Error())
				}
			}
		}

		// ── Comments / mentions / feed ───────────────────────────────────────
		for k := 0; k < len(message.Entry[i].Changes); k++ {
			inbox.IngestComment(sourceId, &message.Entry[i].Changes[k])
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}
