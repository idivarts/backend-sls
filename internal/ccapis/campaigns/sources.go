package campaignsapi

import (
	"net/http"

	"github.com/TrendsHub/th-backend/internal/middlewares"
	"github.com/TrendsHub/th-backend/internal/models"
	"github.com/gin-gonic/gin"
)

// Struct to take input for ConnectSourcesWithCampaign apis. Params is just one sourceId
type IConnectSourcesWithCampaign struct {
	SourceID string `json:"sourceId" binding:"required"`
}

// Create an api to connect sources with the campaign
func ConnectSourcesWithCampaign(c *gin.Context) {
	// Get the campaign id from the url
	campaignID := c.Param("campaignId")

	// Get the sources from the request body
	var req IConnectSourcesWithCampaign
	if err := c.ShouldBindJSON(&req); err != nil {
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
	}

	// // Connect the sources with the campaign
	// err := models.ConnectSourcesWithCampaign(campaignID, sources)
	// if err != nil {
	// 	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	// 	return
	// }

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

	// // Disconnect the sources with the campaign
	// err := models.DisconnectSourcesWithCampaign(campaignID, sources)
	// if err != nil {
	// 	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	// 	return
	// }

	c.JSON(http.StatusOK, gin.H{"message": "Sources disconnected with the campaign"})
}
