package cc_campaigns

// Structs for response structure

type ReplySpeed struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

type ReminderTiming struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

type ChatGPT struct {
	Prescript string `json:"prescript"`
	Purpose   string `json:"purpose"`
	Actor     string `json:"actor"`
	Examples  string `json:"examples"`
}

type Collectible struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Mandatory   bool   `json:"mandatory"`
}

type Reminders struct {
	State            bool   `json:"state"`
	ReminderCount    int    `json:"reminderCount"`
	ReminderExamples string `json:"reminderExamples"`
}

type LeadStage struct {
	Name                 string        `json:"name"`
	Purpose              string        `json:"purpose"`
	Collectibles         []Collectible `json:"collectibles"`
	Reminders            Reminders     `json:"reminders"`
	ExampleConversations string        `json:"exampleConversations"`
	StopConversation     bool          `json:"stopConversation"`
	LeadConversion       bool          `json:"leadConversion"`
}

type CampaignDetails struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Image          string         `json:"image"`
	Objective      string         `json:"objective"`
	ReplySpeed     ReplySpeed     `json:"replySpeed"`
	ReminderTiming ReminderTiming `json:"reminderTiming"`
	ChatGPT        ChatGPT        `json:"chatgpt"`
	LeadStages     []LeadStage    `json:"leadStages"`
}

type CampaignRequest struct {
	Name           string         `json:"name" binding:"required"`
	Image          string         `json:"image"`
	Objective      string         `json:"objective" binding:"required"`
	ReplySpeed     ReplySpeed     `json:"replySpeed" binding:"required,dive"`
	ReminderTiming ReminderTiming `json:"reminderTiming" binding:"required,dive"`
	ChatGPT        ChatGPT        `json:"chatgpt" binding:"required,dive"`
	LeadStages     []LeadStage    `json:"leadStages" binding:"required,dive"`
}

type CampaignResponse struct {
	ID               string `json:"id"`
	Image            string `json:"image"`
	Name             string `json:"name"`
	AssistantID      string `json:"assitantId"`
	TotalLeads       int    `json:"totalLeads"`
	TotalConversions int    `json:"totalConversions"`
	TotalSources     int    `json:"totalSources"`
}
