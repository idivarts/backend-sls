package paymentwebhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/pkg/payments"
)

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
	// You can parse bodyBytes into a map or custom struct based on Razorpay's webhook docs

	// Log or process the event
	// Example (optional):
	// var event map[string]interface{}
	// if err := json.Unmarshal(bodyBytes, &event); err == nil {
	//     fmt.Printf("Received event: %v\n", event)
	// }

	// Acknowledge webhook
	c.JSON(http.StatusOK, gin.H{"status": "Webhook received"})
}

func isValidSignature(body []byte, signature string, secret string) bool {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(body)
	computed := hex.EncodeToString(h.Sum(nil))
	return hmac.Equal([]byte(computed), []byte(signature))
}
