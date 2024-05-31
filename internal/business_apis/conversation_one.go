package businessapis

import (
	"net/http"

	"github.com/TrendsHub/th-backend/internal/models"
	openaifc "github.com/TrendsHub/th-backend/internal/openai/fc"
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
	*models.Conversation
	IGSID       string                `json:"igsid" binding:"required"`
	Information *openaifc.ChangePhase `json:"information,omitempty"`
}

func UpdateConversation(c *gin.Context) {
	var req IUpdateConversation
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// req.
	cData := &models.Conversation{}
	err := cData.Get(req.IGSID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// req.Information

	c.JSON(http.StatusOK, gin.H{"message": "Sync is running in background"})
}
