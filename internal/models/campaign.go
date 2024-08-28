package models

import (
	"context"
	"fmt"

	firestoredb "github.com/TrendsHub/th-backend/pkg/firebase/firestore"
)

type Campaign struct {
	OrganizationID string        `json:"organizationId"`
	Name           string        `json:"name"`
	Objective      string        `json:"objective"`
	CreatedBy      string        `json:"createdBy"`
	CreatedAt      int64         `json:"createdAt"`
	UpdatedAt      int64         `json:"updatedAt"`
	Status         int           `json:"status"`
	ReplySpeed     Range         `json:"replySpeed"`
	ReminderTiming Range         `json:"reminderTiming"`
	ChatGPT        ChatGPTConfig `json:"chatgpt"`

	// This will be used for storing the assistant data
	AssistantID string `json:"assistantId"`

	// LeadStages     []LeadStage   `json:"leadStages"`
}

type Range struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

type ChatGPTConfig struct {
	Prescript string `json:"prescript"`
	Purpose   string `json:"purpose"`
	Actor     string `json:"actor"`
	Examples  string `json:"examples"`
}

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

type Collectible struct {
	OrganizationID string `json:"organizationId"`
	CampaignID     string `json:"campaignId"`
	LeadStageID    string `json:"leadStageId"`
	Name           string `json:"name"`
	Type           string `json:"type"`
	Description    string `json:"description"`
	Mandatory      bool   `json:"mandatory"`
}

func (c *Campaign) Get(organizationId, campaignId string) error {
	iter := firestoredb.Client.Collection(fmt.Sprintf("/organizations/%s/campaigns/%s", organizationId, campaignId)).Documents(context.Background())
	doc, err := iter.Next()
	if err != nil {
		return err
	}
	doc.DataTo(c)
	return nil
}
