package models

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
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
	AssistantID *string `json:"assistantId,omitempty"`

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

func (c *Campaign) Get(organizationId, campaignId string) error {
	doc, err := firestoredb.Client.Collection(fmt.Sprintf("organizations/%s/campaigns", organizationId)).Doc(campaignId).Get(context.Background())
	if err != nil {
		return err
	}
	doc.DataTo(c)
	return nil
}

func (c *Campaign) Update(campaignId string) (*firestore.WriteResult, error) {
	docRef := firestoredb.Client.Collection(fmt.Sprintf("organizations/%s/campaigns", c.OrganizationID)).Doc(campaignId)
	res, err := docRef.Set(context.Background(), c)
	return res, err
}
