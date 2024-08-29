package campaignsapi

import (
	"fmt"
	"net/http"

	"github.com/TrendsHub/th-backend/internal/middlewares"
	"github.com/TrendsHub/th-backend/internal/models"
	"github.com/TrendsHub/th-backend/pkg/openai"
	"github.com/gin-gonic/gin"
)

func createInstruction(campaign *models.Campaign) string {
	// Write the logic to create the instruction script for chatGPT
	markdown := "# Campaign Details\n\n"

	// Add Campaign Name if not empty
	if campaign.Name != "" {
		markdown += fmt.Sprintf("### Campaign Name\n%s\n\n", campaign.Name)
	}

	// Add Objective if not empty
	if campaign.Objective != "" {
		markdown += fmt.Sprintf("### Objective\n%s\n\n", campaign.Objective)
	}

	// Add ChatGPT Configuration if fields are not empty
	chatGPTConfig := ""
	if campaign.ChatGPT.Prescript != "" {
		chatGPTConfig += fmt.Sprintf("### **Prescript:**\n%s\n\n", campaign.ChatGPT.Prescript)
	}
	if campaign.ChatGPT.Purpose != "" {
		chatGPTConfig += fmt.Sprintf("### **Purpose:**\n%s\n\n", campaign.ChatGPT.Purpose)
	}
	if campaign.ChatGPT.Actor != "" {
		chatGPTConfig += fmt.Sprintf("### **Actor:**\n%s\n\n", campaign.ChatGPT.Actor)
	}
	if campaign.ChatGPT.Examples != "" {
		chatGPTConfig += fmt.Sprintf("### **Examples:**\n%s\n", campaign.ChatGPT.Examples)
	}

	if chatGPTConfig != "" {
		markdown += "## ChatGPT Configuration\n" + chatGPTConfig
	}

	return markdown
}

func createToolFunctions(campaign *models.Campaign) []openai.ToolEntry {
	// Write the logic to create the function for chatGPT

	changePhaseFn := openai.ToolEntry{
		Type: openai.TT_FUNCTION,
		Function: openai.Function{
			Name:        "changePhaseFunction",
			Description: "This function will be used whenever we want to change phase",
			Parameters: openai.Parameters{
				Type: openai.PT_OBJECT,
				Properties: map[string]openai.VariableProperty{
					"test": {
						Type:        openai.VT_STRING,
						Enum:        nil,
						Description: "",
					},
				},
				Required: []string{},
			},
		},
	}

	return []openai.ToolEntry{changePhaseFn}
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
