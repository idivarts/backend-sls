package messagewebhook

import (
	"net/http"

	instainterfaces "github.com/TrendsHub/th-backend/pkg/interfaces/instaInterfaces"
	"github.com/gin-gonic/gin"
)

func Receive(c *gin.Context) {
	var message instainterfaces.InstagramMessage
	if err := c.ShouldBindJSON(&message); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Handle the Instagram message as needed
	// You can access message fields like message.Sender.ID, message.Message.Text, etc.

	c.JSON(http.StatusOK, gin.H{
		"message":   "Instagram webhook received successfully",
		"instagram": message,
	})
}
