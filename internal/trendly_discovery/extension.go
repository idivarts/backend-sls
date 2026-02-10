package trendlydiscovery

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlyrdb"
	sqshandler "github.com/idivarts/backend-sls/pkg/sqs_handler"
	"github.com/idivarts/backend-sls/scripts/socials-add-entries/sui"
)

func AddInstaProfile(c *gin.Context) {
	var req sui.ScrapedSocial
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid Input", "error": err.Error()})
		return
	}

	checkData := trendlyrdb.Socials{}
	err := checkData.GetInstagram(req.Username)
	if err == nil {
		c.JSON(http.StatusConflict, gin.H{"message": "Profile already exists", "id": checkData.ID})
		return
	}

	b, err := json.Marshal(&req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to marshal request", "error": err.Error()})
		return
	}
	err = sqshandler.SendToMessageQueue(string(b), 0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to send to queue", "error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"message": "Profile received"})

}

func CheckInstaUsername(c *gin.Context) {
	username := c.Query("username")
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Username is required"})
		return
	}

	user := trendlyrdb.Socials{}
	err := user.GetInstagram(username)
	exists := err == nil
	var lastUpdate int64
	if exists {
		lastUpdate = user.LastUpdateTime
	}

	c.JSON(http.StatusAccepted, gin.H{"username": username, "exists": exists, "lastUpdate": lastUpdate})
}
