package crowdychat

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

// _GetCampaignsQuery holds the query parameters with validation tags
type _GetCampaignsQuery struct {
	Start  int    `form:"start" binding:"required,min=0"`
	Count  int    `form:"count" binding:"required,min=1"`
	Search string `form:"search" binding:"omitempty"`
}

type Campaign struct {
	ID               string `json:"id"`
	Image            string `json:"image"`
	Name             string `json:"name"`
	AssistantID      string `json:"assitantId"`
	TotalLeads       int    `json:"totalLeads"`
	TotalConversions int    `json:"totalConversions"`
	TotalSources     int    `json:"totalSources"`
}

type _CampaignResponse struct {
	Start    int        `json:"start"`
	MoreData bool       `json:"moreData"`
	Content  []Campaign `json:"content"`
}

func GetCampaigns(c *gin.Context) {
	var query _GetCampaignsQuery

	// Validate query parameters
	if err := c.ShouldBindQuery(&query); err != nil {
		validationErrors := err.(validator.ValidationErrors)
		c.JSON(http.StatusBadRequest, gin.H{"error": validationErrors.Error()})
		return
	}

	// Placeholder for actual data retrieval logic, filtering based on 'search'
	// Here we're just mocking some data for the example
	campaigns := []Campaign{
		{
			ID: "391", Image: "https://cdn.fakercloud.com/avatars/SULiik_128.jpg",
			Name: "Unbranded Frozen Soap", AssistantID: "208", TotalLeads: 36, TotalConversions: 64, TotalSources: 936,
		},
		{
			ID: "570", Image: "https://cdn.fakercloud.com/avatars/liminha_128.jpg",
			Name: "Generic Metal Mouse", AssistantID: "546", TotalLeads: 944, TotalConversions: 872, TotalSources: 423,
		},
		{
			ID: "507", Image: "https://cdn.fakercloud.com/avatars/andytlaw_128.jpg",
			Name: "Generic Fresh Computer", AssistantID: "352", TotalLeads: 564, TotalConversions: 530, TotalSources: 172,
		},
		{
			ID: "137", Image: "https://cdn.fakercloud.com/avatars/madcampos_128.jpg",
			Name: "Unbranded Wooden Cheese", AssistantID: "250", TotalLeads: 948, TotalConversions: 988, TotalSources: 669,
		},
	}

	// Apply search filter if provided
	var filteredCampaigns []Campaign
	if query.Search != "" {
		// for _, campaign := range campaigns {
		// 	if contains(campaign.Name, query.Search) {
		// 		filteredCampaigns = append(filteredCampaigns, campaign)
		// 	}
		// }
	} else {
		filteredCampaigns = campaigns
	}

	// Determine if there's more data
	moreData := len(filteredCampaigns) > query.Start+query.Count

	// Slice the campaigns list based on start and count
	if query.Start < len(filteredCampaigns) {
		end := query.Start + query.Count
		if end > len(filteredCampaigns) {
			end = len(filteredCampaigns)
		}
		filteredCampaigns = filteredCampaigns[query.Start:end]
	} else {
		filteredCampaigns = []Campaign{}
	}

	// Create the response
	response := _CampaignResponse{
		Start:    query.Start,
		MoreData: moreData,
		Content:  filteredCampaigns,
	}

	c.JSON(http.StatusOK, Response{
		Data:    response,
		Message: "Success",
	})
}

func GetCampaignByID(c *gin.Context) {
	id := c.Param("id")

	// Placeholder for actual data retrieval logic based on ID
	// Here we're just mocking some data for the example
	campaign := CampaignDetails{
		ID:        id,
		Name:      "Campaign Name 1",
		Image:     "",
		Objective: "",
		ReplySpeed: ReplySpeed{
			Min: 235,
			Max: 3246,
		},
		ReminderTiming: ReminderTiming{
			Min: 235,
			Max: 3246,
		},
		ChatGPT: ChatGPT{
			Prescript: "",
			Purpose:   "",
			Actor:     "",
			Examples:  "",
		},
		LeadStages: []LeadStage{
			{
				Name:    "Stage 1",
				Purpose: "This is an example stage",
				Collectibles: []Collectible{
					{
						Name:        "email",
						Type:        "string",
						Description: "This is the email id of the users",
						Mandatory:   true,
					},
					{
						Name:        "interestedInApp",
						Type:        "boolean",
						Description: "When a user shows interests in the app we mention this as interested",
						Mandatory:   false,
					},
				},
				Reminders: Reminders{
					State:            true,
					ReminderCount:    3,
					ReminderExamples: "",
				},
				ExampleConversations: "Here we will store some example conversations",
				StopConversation:     false,
				LeadConversion:       false,
			},
			{
				Name:    "Stage 2",
				Purpose: "This is an example stage",
				Collectibles: []Collectible{
					{
						Name:        "engagement",
						Type:        "string",
						Description: "This is the email id of the users",
						Mandatory:   true,
					},
					{
						Name:        "engagement_unit",
						Type:        "string",
						Description: "When a user shows interests in the app we mention this as interested",
						Mandatory:   false,
					},
				},
				Reminders: Reminders{
					State:            true,
					ReminderCount:    3,
					ReminderExamples: "",
				},
				ExampleConversations: "Here we will store some example conversations",
				StopConversation:     false,
				LeadConversion:       false,
			},
		},
	}

	c.JSON(http.StatusOK, Response{
		Data:    campaign,
		Message: "Success",
	})
}

func CreateCampaign(c *gin.Context) {
	var request CampaignRequest

	// Validate JSON body
	if err := c.ShouldBindJSON(&request); err != nil {
		validationErrors := err.(validator.ValidationErrors)
		c.JSON(http.StatusBadRequest, gin.H{"error": validationErrors.Error()})
		return
	}

	// Placeholder for actual data creation logic
	// Here we're just mocking some data for the example
	response := CampaignResponse{
		ID:               "959",
		Image:            "https://cdn.fakercloud.com/avatars/ajaxy_ru_128.jpg",
		Name:             "Practical Cotton Shoes",
		AssistantID:      "144",
		TotalLeads:       427,
		TotalConversions: 343,
		TotalSources:     195,
	}

	c.JSON(http.StatusOK, Response{
		Data:    response,
		Message: "Success Data",
	})
}

func UpdateCampaign(c *gin.Context) {
	id := c.Param("id")

	var request CampaignRequest

	// Validate JSON body
	if err := c.ShouldBindJSON(&request); err != nil {
		validationErrors := err.(validator.ValidationErrors)
		c.JSON(http.StatusBadRequest, gin.H{"error": validationErrors.Error()})
		return
	}

	// Placeholder for actual data creation logic
	// Here we're just mocking some data for the example
	response := CampaignResponse{
		ID:               id,
		Image:            "https://cdn.fakercloud.com/avatars/ajaxy_ru_128.jpg",
		Name:             "Practical Cotton Shoes",
		AssistantID:      "144",
		TotalLeads:       427,
		TotalConversions: 343,
		TotalSources:     195,
	}

	c.JSON(http.StatusOK, Response{
		Data:    response,
		Message: "Success Data",
	})
}
