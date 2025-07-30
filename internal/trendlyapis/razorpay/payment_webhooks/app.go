package paymentwebhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/pkg/payments"
)

// Define struct to match Razorpay webhook event
type RazorpayWebhookEvent struct {
	Entity    string   `json:"entity"`
	AccountID string   `json:"account_id"`
	Event     string   `json:"event"`
	Contains  []string `json:"contains"`
	Payload   struct {
		Subscription *struct {
			Entity SubscriptionEntity `json:"entity"`
		} `json:"subscription"`
		Order *struct {
			Entity OrderEntity `json:"entity"`
		} `json:"order"`
		Payment *struct {
			Entity PaymentEntity `json:"entity"`
		} `json:"payment"`
		PaymentLink *struct {
			Entity PaymentLinkEntity `json:"entity"`
		} `json:"payment_link"`
	} `json:"payload"`
	CreatedAt int64 `json:"created_at"`
}

func Handler(c *gin.Context) {
	webhookSecret := payments.WebhookKey
	if webhookSecret == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Webhook secret not configured"})
		return
	}

	// Read request body
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	// Get the Razorpay signature from header
	signature := c.GetHeader("X-Razorpay-Signature")
	if signature == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing signature"})
		return
	}

	// Verify the signature
	if !isValidSignature(bodyBytes, signature, webhookSecret) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid signature"})
		return
	}

	// TODO: Unmarshal JSON and handle event
	var event RazorpayWebhookEvent
	if err := json.Unmarshal(bodyBytes, &event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Failed to parse webhook payload"})
		return
	}

	if strings.HasPrefix(event.Event, "subscription") {
		handleSubscription(event)
	} else if strings.HasPrefix(event.Event, "payment_link") {
		handlePaymentLink(event)
	}

	// Acknowledge webhook
	c.JSON(http.StatusOK, gin.H{"status": "Webhook received"})
}

func isValidSignature(body []byte, signature string, secret string) bool {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(body)
	computed := hex.EncodeToString(h.Sum(nil))
	return hmac.Equal([]byte(computed), []byte(signature))
}
