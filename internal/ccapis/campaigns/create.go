package campaignsapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/TrendsHub/th-backend/internal/middlewares"
	"github.com/TrendsHub/th-backend/internal/models"
	firestoredb "github.com/TrendsHub/th-backend/pkg/firebase/firestore"
	"github.com/TrendsHub/th-backend/pkg/openai"
	"github.com/gin-gonic/gin"
	"google.golang.org/api/iterator"
)

func createInstruction(campaign *models.Campaign, leadStages map[string]models.LeadStage, collectibles map[string]models.Collectible) string {
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

	markdown += "# Conversation Flow\n"
	markdown += "This is a very important section for making converastion with the influencers. The conversation is done by the assistant is always done in phase. There are total 6 phases in conversation. Each phase has different importance and significance. The assistant is not allowed to switch phases in conversation unless it gets the feedback to change phase from the \"change_phase\" function. Each conversation in a thread starts in phase 1 and can move to different phases as the output returned in change_phase function. Below is the explaination of all the phases on conversation\n"

	i := 1
	for lKey, lStage := range leadStages {
		markdown += fmt.Sprintf("## Phase %d - %s\n", i, lStage.Name)
		markdown += (lStage.Purpose + "\n")
		markdown += "Details of the data collected in this phase are listed below\n"

		for _, collectibles := range collectibles {
			if collectibles.LeadStageID == lKey {
				markdown += fmt.Sprintf("- 1. %s - %s\n", collectibles.Name, collectibles.Description)
			}
		}
		i++
	}

	return markdown
}

func createToolFunctions(collectibles map[string]models.Collectible) []openai.ToolEntry {
	// Write the logic to create the function for chatGPT

	properties := map[string]openai.VariableProperty{
		"phase": {
			Type:        openai.VT_NUMBER,
			Enum:        nil,
			Description: "This is a number identifying the current phase of conversation",
		},
	}

	mandatorFields := []string{"phase"}
	for _, data := range collectibles {
		properties[data.Name] = openai.VariableProperty{
			Type:        openai.VariableType(data.Type),
			Enum:        nil,
			Description: data.Description,
		}

		// if data.Mandatory {
		// 	mandatorFields = append(mandatorFields, data.Name)
		// }
	}

	changePhaseFn := openai.ToolEntry{
		Type: openai.TT_FUNCTION,
		Function: openai.Function{
			Name:        "change_phase",
			Description: "This function is called whenever there is a change in phase or addition/updation of any of the data/information to be collected. The purpose of this function is to send and process any collected information from the chat. This function returns two variables missed_information and missed_phases.\n1. missed_information is an array of string that identifies what all information is yet to be collected from the user before they can end the conversation. The assistant need to make sure that it collects all the information in this.\n2. missed_phases is an array of integer identifying is the chat had to skip any phases of conversation. The assistant need to make sure that they cover all the phase mentioned in this return",
			Parameters: openai.Parameters{
				Type:       openai.PT_OBJECT,
				Properties: properties,
				Required:   mandatorFields,
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

	leadStages := map[string]models.LeadStage{}
	iter := firestoredb.Client.CollectionGroup("leadStages").Where("campaignId", "==", campaignId).Documents(context.Background())
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		lS := models.LeadStage{}
		err = doc.DataTo(&lS)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		leadStages[doc.Ref.ID] = lS
	}

	collectibles := map[string]models.Collectible{}
	iter = firestoredb.Client.CollectionGroup("collectibles").Where("campaignId", "==", campaignId).Documents(context.Background())
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		lS := models.Collectible{}
		err = doc.DataTo(&lS)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		collectibles[doc.Ref.ID] = lS
	}

	assistant := openai.CreateAssistantRequest{
		Model:        "gpt-4o",
		Instructions: createInstruction(campaign, leadStages, collectibles),
		Tools:        createToolFunctions(collectibles),
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
