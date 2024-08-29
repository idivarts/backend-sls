package ccapis

import (
	"encoding/json"
	"log"
	"net/http"

	sqsevents "github.com/TrendsHub/th-backend/internal/message_sqs/events"
	"github.com/TrendsHub/th-backend/internal/middlewares"
	"github.com/TrendsHub/th-backend/internal/models"
	"github.com/TrendsHub/th-backend/pkg/messenger"
	sqshandler "github.com/TrendsHub/th-backend/pkg/sqs_handler"
	"github.com/gin-gonic/gin"
)

type IPageWebhook struct {
	Enable *bool `json:"enable" form:"enable" binding:"required"`
}

func PageWebhook(c *gin.Context) {
	var req IPageWebhook
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	organizationID, b := middlewares.GetOrganizationId(c)
	if !b {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No organization in the header"})
		return
	}

	sourceId := c.Param("sourceId")

	cPage := &models.Source{}
	err := cPage.Get(organizationID, sourceId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if cPage.PageID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Page cant be found"})
		return
	}
	err = messenger.SubscribeApp(*req.Enable, *cPage.AccessToken)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cPage.IsWebhookConnected = *req.Enable
	_, err = cPage.Insert()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully parsed JSON"})

}

type IPageSync struct {
	All   bool    `json:"all"`
	IGSID *string `json:"igsid,omitempty"`
}

func PageSync(c *gin.Context) {
	var req IPageSync
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	organizationID, b := middlewares.GetOrganizationId(c)
	if !b {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No organization in the header"})
		return
	}

	sourceId := c.Param("sourceId")
	pData := &models.Source{}
	err := pData.Get(organizationID, sourceId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var conversations []messenger.ConversationMessagesData
	if req.IGSID != nil {
		data, err := messenger.GetConversationsByUserId(*req.IGSID, *pData.AccessToken)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		conversations = data.Data
	} else {
		conversations = messenger.FetchAllConversations(nil, *pData.AccessToken)
	}
	if len(conversations) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No Conversation found"})
		return
	}

	for _, v := range conversations {
		igsid := messenger.GetRecepientIDFromParticipants(v.Participants, *pData.UserName)
		log.Println("IGSID", igsid)
		event := sqsevents.CREATE_THREAD
		if req.All {
			event = sqsevents.CREATE_OR_UPDATE_THREAD
		}
		x := sqsevents.ConversationEvent{
			IGSID:  igsid,
			PageID: sourceId,
			Action: event,
		}
		b, err := json.Marshal(&x)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		sqshandler.SendToMessageQueue(string(b), 0)
	}
	c.JSON(http.StatusOK, gin.H{"message": "Sync is running in background"})
}
