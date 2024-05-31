package businessapis

import (
	"fmt"
	"net/http"

	eventhandling "github.com/TrendsHub/th-backend/internal/message_sqs/event_handling"
	sqsevents "github.com/TrendsHub/th-backend/internal/message_sqs/events"
	"github.com/TrendsHub/th-backend/internal/models"
	"github.com/TrendsHub/th-backend/pkg/messenger"
	"github.com/TrendsHub/th-backend/pkg/openai"
	"github.com/gin-gonic/gin"
)

type IMessagesByID struct {
	IGSID string `form:"igsid" binding:"required"`
	After string `form:"after"`
	Limit int    `form:"limit"`
}

func GetMessages(c *gin.Context) {
	var req IMessagesByID
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

	igConvs, err := messenger.GetConversationsPaginated(req.After, req.Limit, pData.AccessToken)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, *igConvs)
}

type SendType string

const (
	User SendType = "user"
	Bot  SendType = "bot"
	Page SendType = "page"
)

type IStartConversationRequest struct {
	IGSID          string   `json:"igsid" binding:"required"`
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
			IGSID:    req.IGSID,
			ThreadID: cData.ThreadID,
			MID:      cData.LastMID,
		}
		err = eventhandling.RunOpenAI(conv, req.BotInstruction)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	} else if req.SendType == Page && req.Message != "" {
		msg, err := messenger.SendTextMessage(cData.IGSID, req.Message, pData.AccessToken)
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
			IGSID:    req.IGSID,
			ThreadID: cData.ThreadID,
			MID:      cData.LastMID,
		}
		err = eventhandling.RunOpenAI(conv, req.BotInstruction)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Errorf("error : Request invalid - %s, %s, %s", req.SendType, req.Message, req.BotInstruction)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Conversation sent"})
}
