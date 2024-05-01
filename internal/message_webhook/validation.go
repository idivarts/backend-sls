package messagewebhook

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

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

	// request.Mode != "subscribe" ||
	// Validate the request fields as needed
	if request.VerifyToken != "mytoken" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request parameters"})
		return
	}

	// Handle valid request
	c.String(http.StatusOK, "%s", request.Challenge)
}
