package messagewebhook

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

// verifyToken returns the configured webhook verify token, falling back to the
// historical default so existing Meta dashboard subscriptions keep validating.
func verifyToken() string {
	if t := os.Getenv("WEBHOOK_VERIFY_TOKEN"); t != "" {
		return t
	}
	return "mytoken"
}

type WebhookSubscriptionRequest struct {
	Mode        string `form:"hub.mode" binding:"required"`
	VerifyToken string `form:"hub.verify_token" binding:"required"`
	Challenge   string `form:"hub.challenge" binding:"required"`
}

func Validation(c *gin.Context) {
	var request WebhookSubscriptionRequest
	if err := c.ShouldBindQuery(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate the request fields as needed
	if request.Mode != "subscribe" || request.VerifyToken != verifyToken() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request parameters"})
		return
	}

	// Handle valid request
	c.String(http.StatusOK, "%s", request.Challenge)
}
