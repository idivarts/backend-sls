package models

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/TrendsHub/th-backend/pkg/firebase/firestore"
	"github.com/TrendsHub/th-backend/pkg/messenger"
)

type Leads struct {
	ID          string                 `json:"id,omitempty"`
	Email       *string                `json:"email,omitempty"`
	Name        *string                `json:"name,omitempty"`
	SourceType  SourceType             `json:"sourceType"`
	SourceID    string                 `json:"sourceId"`
	UserProfile *messenger.UserProfile `json:"userProfile,omitempty"`
	TagID       *string                `json:"tagId,omitempty"`
	CampaignID  *string                `json:"campaignId,omitempty"`
	Status      int                    `json:"status"`
	CreatedAt   int64                  `json:"createdAt"`
	UpdatedAt   int64                  `json:"updatedAt"`
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
