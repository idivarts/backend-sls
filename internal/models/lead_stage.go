package models

type LeadStage struct {
	OrganizationID       string         `json:"organizationId"`
	CampaignID           string         `json:"campaignId"`
	Name                 string         `json:"name"`
	Purpose              string         `json:"purpose"`
	Reminders            ReminderConfig `json:"reminders"`
	ExampleConversations string         `json:"exampleConversations"`
	StopConversation     bool           `json:"stopConversation"`
	LeadConversion       bool           `json:"leadConversion"`

	// Collectibles         []Collectible  `json:"collectibles"`
}

type ReminderConfig struct {
	State            bool   `json:"state"`
	ReminderCount    int    `json:"reminderCount"`
	ReminderExamples string `json:"reminderExamples"`
}
