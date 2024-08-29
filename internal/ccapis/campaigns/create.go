package campaignsapi

import (
	"net/http"

	"github.com/TrendsHub/th-backend/internal/middlewares"
	"github.com/TrendsHub/th-backend/internal/models"
	"github.com/gin-gonic/gin"
)

func CreateOrUpdateCampaign(c *gin.Context) {
	// var req ISourceSyncLeads
	// if err := c.ShouldBind(&req); err != nil {
	// 	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	// 	return
	// }

	organizationID, b := middlewares.GetOrganizationId(c)
	if !b {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No organization in the header"})
		return
	}

	campaignId := c.Param("campaignId")

	campaign := &models.Campaign{}
	err := campaign.Get(organizationID, campaignId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Write the logic to create the script for chatGPT

	// Write the logic to create the function for chatGPT

	// Write logic in openai to either update or create new assistant

	c.JSON(http.StatusOK, gin.H{"message": "Create Done"})
}
