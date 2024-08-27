package models

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/TrendsHub/th-backend/pkg/firebase/firestore"
	"google.golang.org/api/iterator"
)

type SourceType string

const (
	Facebook  SourceType = "facebook"
	Instagram SourceType = "instagram"
	YouTube   SourceType = "youtube"
	Email     SourceType = "email"
)

//	type InstagramObject struct {
//		ID       string `json:"id" dynamodbav:"id"`
//		Name     string `json:"name" dynamodbav:"name"`
//		UserName string `json:"userName" dynamodbav:"userName"`
//		Bio      string `json:"bio" dynamodbav:"bio"`
//	}
type Source struct {
	OrganizationID     string     `json:"organizationId"`
	PageID             string     `json:"pageId"`
	Name               string     `json:"name"`
	UserID             string     `json:"userId"`
	OwnerName          string     `json:"ownerName"`
	IsWebhookConnected bool       `json:"isWebhookConnected"`
	Status             int        `json:"status"`
	UserName           *string    `json:"userName,omitempty"`
	Bio                *string    `json:"bio,omitempty"`
	SourceType         SourceType `json:"sourceType"`
	ConnectedID        *string    `json:"connectedId,omitempty"`
	AccessToken        *string    `json:"accessToken,omitempty"`

	// OLD FIELDS that we would need to shift in a different model
	IsInstagram            bool   `json:"isInstagram" dynamodbav:"isInstagram"`
	AssistantID            string `json:"assistantId" dynamodbav:"assistantId"`
	ReminderTimeMultiplier int    `json:"reminderTimeMultiplier" dynamodbav:"reminderTimeMultiplier"`
	ReplyTimeMin           int    `json:"replyTimeMin" dynamodbav:"replyTimeMin"`
	ReplyTimeMax           int    `json:"replyTimeMax" dynamodbav:"replyTimeMax"`

	// Instagram   *InstagramObject `json:"instagram,omitempty"`
}

func (c *Source) GetPath() (*string, error) {
	if c.OrganizationID == "" {
		return nil, fmt.Errorf("Organzation(%s) cant be null", c.OrganizationID)
	}

	path := fmt.Sprintf("/organizations/%s/sources", c.OrganizationID)
	return &path, nil
}

func (c *Source) Insert() (*firestore.WriteResult, error) {
	path, err := c.GetPath()
	if err != nil {
		return nil, err
	}

	res, err := firestoredb.Client.Collection(*path).Doc(c.PageID).Set(context.Background(), c)
	return res, err
}

func (c *Source) Get(pageId string) error {
	path, err := c.GetPath()
	if err != nil {
		return err
	}

	result, err := firestoredb.Client.Collection(*path).Doc(pageId).Get(context.Background())
	if err != nil {
		fmt.Println("Error getting item from Firestore:", err)
		return err
	}

	err = result.DataTo(c)
	if err != nil {
		fmt.Println("Error getting item from Firestore:", err)
		return err
	}

	return nil
}

func GetPagesByUserId(userId string) ([]Source, error) {

	sources := []Source{}
	iter := firestoredb.Client.CollectionGroup("sources").Where("userId", "==", userId).Documents(context.Background())

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		source := Source{}
		err = doc.DataTo(&source)
		if err != nil {
			continue
		}
		sources = append(sources, source)
	}

	return sources, nil
}

func FetchAllPages() ([]Source, error) {
	sources := []Source{}
	iter := firestoredb.Client.CollectionGroup("sources").Documents(context.Background())

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		source := Source{}
		err = doc.DataTo(&source)
		if err != nil {
			continue
		}
		sources = append(sources, source)
	}

	return sources, nil
}
