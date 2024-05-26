package businessapis

import (
	"net/http"

	"github.com/TrendsHub/th-backend/internal/models"
	"github.com/TrendsHub/th-backend/pkg/messenger"
	"github.com/gin-gonic/gin"
)

type IPageWebhook struct {
	Enable bool `json:"enable" binding:"required"`
}

func PageWebhook(c *gin.Context) {
	var req IPageWebhook
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	pageId := c.Param("pageId")

	cPage := &models.Page{}
	err := cPage.Get(pageId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if cPage.PageID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Page cant be found"})
		return
	}
	err = messenger.SubscribeApp(req.Enable, cPage.AccessToken)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cPage.IsWebhookConnected = req.Enable
	_, err = cPage.Insert()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully parsed JSON"})

}

type IPageAssistant struct {
	AssistantID            string `json:"assistantId" binding:"required"`
	ReminderTimeMultiplier int    `json:"reminderTimeMultiplier"`
	ReplyTimeMin           int    `json:"replyTimeMin"`
	ReplyTimeMax           int    `json:"replyTimeMax"`
}

func PageAssistant(c *gin.Context) {
	var req IPageAssistant
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	pageId := c.Param("pageId")

	cPage := &models.Page{}
	err := cPage.Get(pageId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if cPage.PageID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Page cant be found"})
		return
	}

	cPage.AssistantID = req.AssistantID
	cPage.ReminderTimeMultiplier = req.ReminderTimeMultiplier
	cPage.ReplyTimeMax = req.ReplyTimeMax
	cPage.ReplyTimeMin = req.ReplyTimeMin

	_, err = cPage.Insert()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully parsed JSON"})

}

func PageSync(c *gin.Context) {

}
