package sourcesapi

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models"
	"github.com/idivarts/backend-sls/pkg/messenger"
)

type IPageWebhook struct {
	Enable *bool `json:"enable" form:"enable" binding:"required"`
}

func SourceWebhookAction(c *gin.Context) {
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
	sP := &models.SourcePrivate{}
	err = sP.Get(organizationID, sourceId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if cPage.ID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Page cant be found"})
		return
	}
	err = messenger.SubscribeApp(*req.Enable, *sP.AccessToken)
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

type ISourceSyncLeads struct {
	TagID *string `json:"tagId,omitempty"`
}

func SourceSyncLeads(c *gin.Context) {
	var req ISourceSyncLeads
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

	sData := &models.Source{}
	err := sData.Get(organizationID, sourceId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	sP := &models.SourcePrivate{}
	err = sP.Get(organizationID, sourceId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var conversations []messenger.ConversationMessagesData = messenger.FetchAllConversations(nil, *sP.AccessToken)

	if len(conversations) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No Conversation found"})
		return
	}

	for _, v := range conversations {
		igsid := messenger.GetRecepientIDFromParticipants(v.Participants, *sData.UserName)
		log.Println("IGSID", igsid)

		uProfile, err := messenger.GetUser(igsid, *sP.AccessToken)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		lead := &models.Leads{
			ID:          igsid,
			Email:       nil,
			Name:        &uProfile.Name,
			SourceType:  sData.SourceType,
			SourceID:    sData.ID,
			UserProfile: uProfile,
			TagID:       req.TagID,
			CampaignID:  nil,
			Status:      1,
			CreatedAt:   time.Now().UnixMilli(),
			UpdatedAt:   time.Now().UnixMilli(),
		}
		_, err = lead.Insert(organizationID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"message": "Sync is running in background"})
}
