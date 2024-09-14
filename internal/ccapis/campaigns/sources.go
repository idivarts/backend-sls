package campaignsapi

import (
	"net/http"

	"github.com/TrendsHub/th-backend/internal/middlewares"
	"github.com/TrendsHub/th-backend/internal/models"
	"github.com/gin-gonic/gin"
)

func removeAllConversationFromCampaign(organizationID, campaignID, sourceID string) error {
	// Get all the conversations of the source
	conversations, err := models.GetConversations(organizationID, campaignID, &sourceID, nil)
	if err != nil {
		return err
	}

	// Remove all the conversations from the campaign
	for _, conversation := range conversations {
		conversation.Status = 30
		_, err := conversation.Insert()
		if err != nil {
			return err
		}
	}

	return nil
}

func addAllConversationToCampaign(organizationID, campaignID, sourceID string) error {
	leads, err := models.GetLeads(organizationID, sourceID)
	if err != nil {
		return err
	}

	for _, lead := range leads {
		conversation := models.Conversation{
			LeadID:           lead.ID,
			OrganizationID:   organizationID,
			CampaignID:       campaignID,
			SourceID:         sourceID,
			IsProfileFetched: lead.UserProfile != nil,

			ThreadID:           "",
			LastMID:            "",
			LastBotMessageTime: 0,
			BotMessageCount:    0,
			CurrentPhase:       0,
			ReminderCount:      0,
			Phases:             []int{},
			Collectibles:       map[string]string{},
			MessageQueue:       nil,
			NextMessageTime:    nil,
			NextReminderTime:   nil,
			ReminderQueue:      nil,

			Status: 1,
		}
		_, err := conversation.Insert()
		if err != nil {
			return err
		}
	}

	return nil
}

// Struct to take input for ConnectSourcesWithCampaign apis. Params is just one sourceId
type IConnectSourcesWithCampaign struct {
	SourceID string `json:"sourceId" form:"sourceId" binding:"required"`
}

// Create an api to connect sources with the campaign
func ConnectSourcesWithCampaign(c *gin.Context) {
	// Get the campaign id from the url
	campaignID := c.Param("campaignId")

	// Get the sources from the request body
	var req IConnectSourcesWithCampaign
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get Organization ID from the header
	organizationID, b := middlewares.GetOrganizationId(c)
	if !b {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No organization in the header"})
		return
	}

	source := models.Source{}
	err := source.Get(organizationID, req.SourceID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	campaign := models.Campaign{}
	err = campaign.Get(organizationID, campaignID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if source.CampaignID != nil {
		// Write code here to remove the source from the previous campaign
		err = removeAllConversationFromCampaign(organizationID, campaignID, req.SourceID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	source.CampaignID = &campaignID
	_, err = source.Insert()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err = addAllConversationToCampaign(organizationID, campaignID, req.SourceID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Sources connected with the campaign"})
}

// Struct to take input for DisconnectSourcesWithCampaign apis. Params is just one sourceId
type IDisconnectSourcesFromCampaign struct {
	SourceID string `form:"sourceId" binding:"required"`
}

// Create an api to disconnect sources with the campaign
func DisconnectSourcesFromCampaign(c *gin.Context) {
	// Get the campaign id from the url
	campaignID := c.Param("campaignId")

	// Get the sources from the request body
	var req IDisconnectSourcesFromCampaign
	if err := c.ShouldBind(&req); err != nil {

		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get Organization ID from the header
	organizationID, b := middlewares.GetOrganizationId(c)
	if !b {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No organization in the header"})
		return
	}

	source := models.Source{}
	err := source.Get(organizationID, req.SourceID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	campaign := models.Campaign{}
	err = campaign.Get(organizationID, campaignID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if source.CampaignID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Source is not connected with any campaign"})
		return
	}

	// Write code here to remove the source from the previous campaign
	err = removeAllConversationFromCampaign(organizationID, campaignID, req.SourceID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	source.CampaignID = nil
	_, err = source.Insert()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Sources disconnected with the campaign"})
}
