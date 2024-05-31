package businessapis

import (
	"net/http"

	"github.com/TrendsHub/th-backend/internal/models"
	"github.com/gin-gonic/gin"
)

type IConversationByID struct {
	IGSID string `form:"igsid" binding:"required"`
}

func GetConversationById(c *gin.Context) {
	var req IConversationByID
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cData := &models.Conversation{}
	err := cData.Get(req.IGSID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	pData := &models.Page{}
	err = pData.Get(cData.PageID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	pData.AccessToken = ""
	c.JSON(http.StatusOK, gin.H{"message": "Sync is running in background", "conversation": *cData, "page": *pData})
}

type IUpdateConversation struct {
	IGSID string `json:"igsid" binding:"required"`
}

func UpdateConversation(c *gin.Context) {
	var req IUpdateConversation
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// messenger.GetConversationsPaginated()

	c.JSON(http.StatusOK, gin.H{"message": "Sync is running in background"})
}
