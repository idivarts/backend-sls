package ccapis

import (
	"net/http"

	eventhandling "github.com/TrendsHub/th-backend/internal/message_sqs/event_handling"
	sqsevents "github.com/TrendsHub/th-backend/internal/message_sqs/events"
	"github.com/TrendsHub/th-backend/internal/middlewares"
	"github.com/TrendsHub/th-backend/internal/models"
	"github.com/TrendsHub/th-backend/pkg/messenger"
	"github.com/TrendsHub/th-backend/pkg/openai"
	"github.com/gin-gonic/gin"
)

type IMessagesByID struct {
	After string `form:"after"`
	Limit int    `form:"limit"`
}

func GetMessages(c *gin.Context) {
	var req IMessagesByID
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	organizationID, b := middlewares.GetOrganizationId(c)
	if !b {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No organization in the header"})
		return
	}

	conversationID := c.Param("conversationId")

	cData := &models.Conversation{}
	err := cData.Get(organizationID, conversationID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	pData := &models.Source{}
	err = pData.Get(organizationID, cData.SourceID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	igConvs, err := messenger.GetConversationsByUserId(cData.LeadID, *pData.AccessToken)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(igConvs.Data) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No conversation found"})
		return
	}
	convId := igConvs.Data[0].ID
	messages, err := messenger.GetMessagesWithPagination(convId, req.After, req.Limit, *pData.AccessToken)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, *messages)
}

type SendType string

const (
	User SendType = "user"
	Bot  SendType = "bot"
	Page SendType = "page"
)

type IStartConversationRequest struct {
	SendType       SendType `json:"sendType" binding:"required"`
	Message        string   `json:"message"`
	BotInstruction string   `json:"botInstruction"`
}

func SendMessage(c *gin.Context) {
	var req IStartConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	organizationID, b := middlewares.GetOrganizationId(c)
	if !b {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No organization in the header"})
		return
	}

	conversationID := c.Param("conversationId")

	cData := &models.Conversation{}
	err := cData.Get(organizationID, conversationID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	pData := &models.Source{}
	err = pData.Get(organizationID, cData.SourceID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.SendType == User && req.Message != "" {
		_, err = openai.SendMessage(cData.ThreadID, req.Message, nil, false)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if len(cData.Phases) > 0 {
			cData.CurrentPhase = cData.Phases[len(cData.Phases)-1]
		}
		_, err = cData.Insert()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		// openai.SendMessage()
		conv := &sqsevents.ConversationEvent{
			Action:   sqsevents.RUN_OPENAI,
			IGSID:    cData.LeadID,
			ThreadID: cData.ThreadID,
			MID:      cData.LastMID,
		}
		err = eventhandling.RunOpenAI(conv, req.BotInstruction)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	} else if req.SendType == Page && req.Message != "" {
		msg, err := messenger.SendTextMessage(cData.LeadID, req.Message, *pData.AccessToken)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		// No need to send message on open ai as that would be automatically processed in the webhook loop

		cData.LastMID = msg.MessageID
		_, err = cData.Insert()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	} else if req.SendType == Bot && req.BotInstruction != "" {
		conv := &sqsevents.ConversationEvent{
			Action:   sqsevents.RUN_OPENAI,
			IGSID:    cData.LeadID,
			ThreadID: cData.ThreadID,
			MID:      cData.LastMID,
		}
		err = eventhandling.RunOpenAI(conv, req.BotInstruction)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Request invalid", "request": req})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Conversation sent"})
}
