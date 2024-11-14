package models

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"github.com/idivarts/backend-sls/pkg/messenger"
)

type Leads struct {
	ID          string                 `json:"id,omitempty" firestore:"id"`
	Email       *string                `json:"email,omitempty" firestore:"email"`
	Name        *string                `json:"name,omitempty" firestore:"name"`
	SourceType  SourceType             `json:"sourceType" firestore:"sourceType"`
	SourceID    string                 `json:"sourceId" firestore:"sourceId"`
	UserProfile *messenger.UserProfile `json:"userProfile,omitempty" firestore:"userProfile"`
	TagID       *string                `json:"tagId,omitempty" firestore:"tagId"`
	CampaignID  *string                `json:"campaignId,omitempty" firestore:"campaignId"`
	Status      int                    `json:"status" firestore:"status"`
	CreatedAt   int64                  `json:"createdAt" firestore:"createdAt"`
	UpdatedAt   int64                  `json:"updatedAt" firestore:"updatedAt"`
}

func (c *Leads) GetPath(organizationID string) (*string, error) {
	path := fmt.Sprintf("organizations/%s/leads", organizationID)
	return &path, nil
}

func (c *Leads) Insert(organizationID string) (*firestore.WriteResult, error) {
	path, err := c.GetPath(organizationID)
	if err != nil {
		return nil, err
	}

	res, err := firestoredb.Client.Collection(*path).Doc(c.ID).Set(context.Background(), c)
	return res, err
}

// This function will get all leads for a given source from firestore
func GetLeads(organizationID, sourceID string) ([]Leads, error) {
	var leads []Leads

	iter := firestoredb.Client.Collection(fmt.Sprintf("organizations/%s/leads", organizationID)).Where("sourceId", "==", sourceID).Documents(context.Background())
	for {
		doc, err := iter.Next()
		if err != nil {
			break
		}

		var lead Leads
		doc.DataTo(&lead)
		leads = append(leads, lead)
	}

	return leads, nil
}
