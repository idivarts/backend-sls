package ccapis

import (
	"net/http"

	"github.com/TrendsHub/th-backend/internal/models"
	delayedsqs "github.com/TrendsHub/th-backend/pkg/delayed_sqs"
	"github.com/gin-gonic/gin"
)

type IUpdateConversation struct {
	models.Conversation
	Status *int `json:"status,omitempty"`
}

func StopConversation(c *gin.Context) {
	leadId := c.Param("leadId")

	cData := &models.Conversation{}
	err := cData.Get(leadId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cData.Status = 0

	delayedsqs.StopExecutions(cData.ReminderQueue)
	cData.ReminderQueue = nil
	cData.NextReminderTime = nil

	delayedsqs.StopExecutions(cData.MessageQueue)
	cData.MessageQueue = nil
	cData.NextMessageTime = nil

	_, err = cData.Insert()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Update done", "conversation": *cData})
}
