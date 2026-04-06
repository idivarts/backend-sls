package paymentwebhooks

import (
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/pkg/payments"
	"github.com/idivarts/backend-sls/pkg/payments/webhook"
)

func Handler(c *gin.Context) {
	webhookSecret := payments.WebhookKey
	if webhookSecret == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Webhook secret not configured"})
		return
	}

	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	signature := c.GetHeader("X-Razorpay-Signature")
	if signature == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing signature"})
		return
	}

	event, err := webhook.VerifyAndParse(bodyBytes, signature, webhookSecret)
	if err != nil {
		if errors.Is(err, webhook.ErrInvalidSignature) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid signature"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Webhook failed to parse, but acknowledged", "error": err.Error()})
		return
	}

	log.Println("Received Event", string(bodyBytes))

	if strings.HasPrefix(event.Event, "subscription") {
		err := HandleSubscription(event)
		if err != nil {
			log.Println("Error", gin.H{"message": "Subscription payload not processed", "error": err.Error(), "event": event})
			c.JSON(http.StatusBadRequest, gin.H{"message": "Subscription payload not processed", "error": err.Error(), "event": event})
			return
		}
	} else if strings.HasPrefix(event.Event, "payment_link") {
		err := handlePaymentLink(event)
		if err != nil {
			log.Println("Error", gin.H{"message": "Payment Link payload not processed", "error": err.Error(), "event": event})
			c.JSON(http.StatusBadRequest, gin.H{"message": "Payment Link payload not processed", "error": err.Error(), "event": event})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "Webhook received"})
}
