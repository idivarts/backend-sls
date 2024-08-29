package ccapis

import (
	"net/http"

	"github.com/TrendsHub/th-backend/internal/middlewares"
	"github.com/TrendsHub/th-backend/internal/models"
	delayedsqs "github.com/TrendsHub/th-backend/pkg/delayed_sqs"
	"github.com/gin-gonic/gin"
)

type IUpdateConversation struct {
	models.Conversation
	Status *int `json:"status,omitempty"`
}

func UpdateConversation(c *gin.Context) {
	leadId := c.Param("leadId")

	organizationID, b := middlewares.GetOrganizationId(c)
	if !b {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No organization in the header"})
		return
	}

	cData := &models.Conversation{}
	err := cData.Get(organizationID, leadId)
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
