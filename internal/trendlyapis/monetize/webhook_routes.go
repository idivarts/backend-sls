package monetize

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/pkg/payments"
)

type RazorpayRouteEvent struct {
	Entity    string                 `json:"entity"`
	AccountID string                 `json:"account_id"`
	Event     string                 `json:"event"`
	Payload   map[string]interface{} `json:"payload"`
}

func RouteWebhook(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Printf("Failed to read webhook body: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
		return
	}

	signature := c.GetHeader("X-Razorpay-Signature")
	if !payments.VerifyWebhookSignature(body, signature, payments.WebhookKey) {
		log.Printf("Invalid Razorpay signature: %s", signature)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
		return
	}

	var event RazorpayRouteEvent
	if err := json.Unmarshal(body, &event); err != nil {
		log.Printf("Failed to unmarshal Razorpay event: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to unmarshal event"})
		return
	}

	log.Printf("Received Razorpay Transfer Event: %s", event.Event)

	// Get this done

	c.JSON(http.StatusOK, gin.H{"status": "received"})
}
