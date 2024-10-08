package conversationsapi

import (
	"net/http"

	"github.com/TrendsHub/th-backend/internal/middlewares"
	"github.com/TrendsHub/th-backend/internal/models"
	delayedsqs "github.com/TrendsHub/th-backend/pkg/delayed_sqs"
	"github.com/gin-gonic/gin"
)

type IUpdateConversation struct {
	models.Conversation
	// Information  *openaifc.ChangePhase `json:"information,omitempty"`
	CurrentPhase *int `json:"currentPhase,omitempty"`
	Status       *int `json:"status,omitempty"`
}

func UpdateConversation(c *gin.Context) {
	var req IUpdateConversation
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	campaignID := c.Param("campaignId")
	conversationID := c.Param("conversationId")

	organizationID, b := middlewares.GetOrganizationId(c)
	if !b {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No organization in the header"})
		return
	}

	cData := &models.Conversation{}
	err := cData.Get(organizationID, campaignID, conversationID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// if req.Information != nil {
	// 	cData.Information = *req.Information
	// }
	// if req.SourceID != "" {
	// 	cData.SourceID = req.SourceID
	// }
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
	// if req.UserProfile != nil {
	// 	cData.UserProfile = req.UserProfile
	// }
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
