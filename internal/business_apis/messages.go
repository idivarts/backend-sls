package businessapis

import (
	"net/http"

	eventhandling "github.com/TrendsHub/th-backend/internal/message_sqs/event_handling"
	sqsevents "github.com/TrendsHub/th-backend/internal/message_sqs/events"
	"github.com/TrendsHub/th-backend/internal/models"
	"github.com/TrendsHub/th-backend/pkg/openai"
	"github.com/gin-gonic/gin"
)

func GetMessages(c *gin.Context) {

}

type IStartConversationRequest struct {
	IGSID                 string `json:"igsid" binding:"required"`
	Message               string `json:"Message" binding:"required"`
	AdditionalInstruction string `json:"AdditionalInstruction"`
}

func SendMessage(c *gin.Context) {
	var req IStartConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cData := &models.Conversation{}
	err := cData.Get(req.IGSID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	_, err = openai.SendMessage(cData.ThreadID, req.Message, nil, false)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cData.IsConversationPaused = 0
	_, err = cData.Insert()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// openai.SendMessage()
	conv := &sqsevents.ConversationEvent{
		Action:   sqsevents.RUN_OPENAI,
		IGSID:    req.IGSID,
		ThreadID: cData.ThreadID,
		MID:      cData.LastMID,
	}
	err = eventhandling.RunOpenAI(conv, req.AdditionalInstruction)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Conversation started successfully"})
}