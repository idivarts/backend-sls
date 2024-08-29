package campaignsapi

import (
	"net/http"

	"github.com/TrendsHub/th-backend/internal/middlewares"
	"github.com/TrendsHub/th-backend/internal/models"
	"github.com/TrendsHub/th-backend/pkg/openai"
	"github.com/gin-gonic/gin"
)

func createInstruction(campaign *models.Campaign) string {
	// Write the logic to create the instruction script for chatGPT
	return ""
}

func createToolFunctions(campaign *models.Campaign) []openai.ToolEntry {
	// Write the logic to create the function for chatGPT
	return []openai.ToolEntry{}
}

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

	assistant := openai.CreateAssistantRequest{
		Model:        "gpt-4o",
		Instructions: createInstruction(campaign),
		Tools:        createToolFunctions(campaign),
	}

	// Write logic in openai to either update or create new assistant
	if campaign.AssistantID != nil {
		_, err = openai.UpdateAssistant(*campaign.AssistantID, assistant)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	} else {
		rC, err := openai.CreateAssistant(assistant)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		campaign.AssistantID = &rC.AssistantID
		_, err = campaign.Update(campaignId)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Create Done"})
}
