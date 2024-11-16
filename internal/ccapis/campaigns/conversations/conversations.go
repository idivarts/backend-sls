package conversationsapi

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	sqsevents "github.com/idivarts/backend-sls/internal/message_sqs/events"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models"
	"github.com/idivarts/backend-sls/pkg/messenger"
	sqshandler "github.com/idivarts/backend-sls/pkg/sqs_handler"
)

// type ISyncConversations struct {
// 	ConversationID string `json:"conversationId"`
// }

func SyncConversations(c *gin.Context) {
	// var req ISyncConversations
	// if err := c.ShouldBind(&req); err != nil {
	// 	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	// 	return
	// }

	organizationID, b := middlewares.GetOrganizationId(c)
	if !b {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No organization in the header"})
		return
	}

	campaignID := c.Param("campaignId")
	conversationID := c.Param("conversationId")

	cData := &models.Conversation{}
	err := cData.Get(organizationID, campaignID, conversationID)
	// cDatas, err := models.GetConversations(organizationID, nil, nil)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// pDatas, err := models.FetchAllPages(organizationID)
	// if err != nil {
	// 	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	// 	return
	// }

	// pDataMap := map[string]models.Source{}
	// for _, sr := range pDatas {
	// 	pDataMap[sr.ID] = sr
	// }

	pData := &models.Source{}
	err = pData.Get(organizationID, cData.SourceID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ppData := &models.SourcePrivate{}
	err = ppData.Get(organizationID, cData.SourceID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var conversations []messenger.ConversationMessagesData
	data, err := messenger.GetConversationsByUserId(cData.LeadID, *ppData.AccessToken)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	conversations = data.Data
	// if req.IGSID != nil {

	// } else {
	// 	conversations = messenger.FetchAllConversations(nil, *pData.AccessToken)
	// }
	if len(conversations) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No Conversation found"})
		return
	}

	for _, v := range conversations {
		igsid := messenger.GetRecepientIDFromParticipants(v.Participants, *pData.UserName)
		log.Println("IGSID", igsid)
		event := sqsevents.CREATE_THREAD
		// if req.All {
		// 	event = sqsevents.CREATE_OR_UPDATE_THREAD
		// }
		x := sqsevents.ConversationEvent{
			LeadID:   igsid,
			SourceID: cData.SourceID,
			Action:   event,
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
