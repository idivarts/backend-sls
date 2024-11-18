package models

type LeadStage struct {
	OrganizationID       string         `json:"organizationId" firestore:"organizationId"`
	CampaignID           string         `json:"campaignId" firestore:"campaignId"`
	Name                 string         `json:"name" firestore:"name"`
	Purpose              string         `json:"purpose" firestore:"purpose"`
	Reminders            ReminderConfig `json:"reminders" firestore:"reminders"`
	ExampleConversations string         `json:"exampleConversations" firestore:"exampleConversations"`
	StopConversation     bool           `json:"stopConversation" firestore:"stopConversation"`
	LeadConversion       bool           `json:"leadConversion" firestore:"leadConversion"`
	Index                int            `json:"index" firestore:"index"`

	// Collectibles         []Collectible  `json:"collectibles"`
}

type ReminderConfig struct {
	State            bool   `json:"state" firestore:"state"`
	ReminderCount    int    `json:"reminderCount" firestore:"reminderCount"`
	ReminderExamples string `json:"reminderExamples" firestore:"reminderExamples"`
}
