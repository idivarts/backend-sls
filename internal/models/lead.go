package models

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/TrendsHub/th-backend/pkg/firebase/firestore"
	"github.com/TrendsHub/th-backend/pkg/messenger"
)

type Lead struct {
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

func (c *Lead) GetPath(organizationID string) (*string, error) {
	path := fmt.Sprintf("organizations/%s/leads", organizationID)
	return &path, nil
}

func (c *Lead) Insert(organizationID string) (*firestore.WriteResult, error) {
	path, err := c.GetPath(organizationID)
	if err != nil {
		return nil, err
	}

	res, err := firestoredb.Client.Collection(*path).Doc(c.ID).Set(context.Background(), c)
	return res, err
}

func (c *Lead) Get(organizationID string, leadID string) error {
	path, err := c.GetPath(organizationID)
	if err != nil {
		return err
	}

	res, err := firestoredb.Client.Collection(*path).Doc(c.ID).Get(context.Background())
	if err != nil {
		return err
	}
	err = res.DataTo(c)
	if err != nil {
		return err
	}
	return nil
}

// This function will get all leads for a given source from firestore
func GetLeads(organizationID, sourceID string) ([]Lead, error) {
	var leads []Lead

	iter := firestoredb.Client.Collection(fmt.Sprintf("organizations/%s/leads", organizationID)).Where("sourceId", "==", sourceID).Documents(context.Background())
	for {
		doc, err := iter.Next()
		if err != nil {
			break
		}

		var lead Lead
		doc.DataTo(&lead)
		leads = append(leads, lead)
	}

	return leads, nil
}
