package businessapis

import (
	"net/http"

	"github.com/TrendsHub/th-backend/internal/models"
	openaifc "github.com/TrendsHub/th-backend/internal/openai/fc"
	delayedsqs "github.com/TrendsHub/th-backend/pkg/delayed_sqs"
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
	IGSID        string                `json:"igsid" binding:"required"`
	Information  *openaifc.ChangePhase `json:"information,omitempty"`
	CurrentPhase *int                  `json:"currentPhase,omitempty"`
	Status       *int                  `json:"status,omitempty"`
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
	if req.Information != nil {
		cData.Information = *req.Information
	}
	if req.PageID != "" {
		cData.PageID = req.PageID
	}
	if req.CurrentPhase != nil {
		cData.CurrentPhase = *req.CurrentPhase
	}
	if req.Status != nil {
		cData.Status = *req.Status
	}
	if req.ReminderQueue != nil {
		delayedsqs.StopExecutions(req.ReminderQueue)
		if *req.ReminderQueue == *cData.ReminderQueue {
			cData.ReminderQueue = nil
			cData.NextReminderTime = nil
		}
	}
	if req.MessageQueue != nil {
		delayedsqs.StopExecutions(req.MessageQueue)
		if *req.MessageQueue == *cData.MessageQueue {
			cData.MessageQueue = nil
			cData.NextMessageTime = nil
		}
	}
	if req.UserProfile != nil {
		cData.UserProfile = req.UserProfile
	}
	// if req. != nil {
	// 	cData.CurrentPhase = *req.CurrentPhase
	// }
	_, err = cData.Insert()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Update done", "conversation": *cData})
}
