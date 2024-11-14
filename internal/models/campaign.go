package models

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

type Campaign struct {
	OrganizationID string        `json:"organizationId" firestore:"organizationId"`
	Name           string        `json:"name" firestore:"name"`
	Objective      string        `json:"objective" firestore:"objective"`
	CreatedBy      string        `json:"createdBy" firestore:"createdBy"`
	CreatedAt      int64         `json:"createdAt" firestore:"createdAt"`
	UpdatedAt      int64         `json:"updatedAt" firestore:"updatedAt"`
	Status         int           `json:"status" firestore:"status"`
	ReplySpeed     Range         `json:"replySpeed" firestore:"replySpeed"`
	ReminderTiming Range         `json:"reminderTiming" firestore:"reminderTiming"`
	ChatGPT        ChatGPTConfig `json:"chatgpt" firestore:"chatgpt"`

	// This will be used for storing the assistant data
	AssistantID *string `json:"assistantId,omitempty" firestore:"assistantId"`

	// LeadStages     []LeadStage   `json:"leadStages"`
}

type Range struct {
	Min int `json:"min" firestore:"min"`
	Max int `json:"max" firestore:"max"`
}

type ChatGPTConfig struct {
	Prescript string `json:"prescript" firestore:"prescript"`
	Purpose   string `json:"purpose" firestore:"purpose"`
	Actor     string `json:"actor" firestore:"actor"`
	Examples  string `json:"examples" firestore:"examples"`
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
