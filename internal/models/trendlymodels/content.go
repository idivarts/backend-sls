package trendlymodels

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

// Content mirrors the brand-app content document at
// brands/{brandId}/contents/{contentId}. Only the fields the backend
// publishing pipeline needs are modelled here.

type ContentAttachment struct {
	Type     string `json:"type" firestore:"type"`
	ImageURL string `json:"imageUrl,omitempty" firestore:"imageUrl"`
	PlayURL  string `json:"playUrl,omitempty" firestore:"playUrl"`
	AppleURL string `json:"appleUrl,omitempty" firestore:"appleUrl"`
}

type ContentDestination struct {
	SocialAccountID string `json:"socialAccountId" firestore:"socialAccountId"`
	Platform        string `json:"platform" firestore:"platform"`
	Username        string `json:"username,omitempty" firestore:"username"`
}

// ContentImageGeneration tracks the live state of an AI image-generation job on
// the content doc. It is written by the websocket image handler so the brand app
// can render progress and the finished image from its Firestore subscription —
// independent of the websocket connection that kicked the job off.
type ContentImageGeneration struct {
	Status         string `json:"status" firestore:"status"` // "generating" | "done" | "error"
	Prompt         string `json:"prompt,omitempty" firestore:"prompt"`
	Error          string `json:"error,omitempty" firestore:"error"`
	RequestedCount int    `json:"requestedCount,omitempty" firestore:"requestedCount"`
	CompletedCount int    `json:"completedCount,omitempty" firestore:"completedCount"`
	StartedAt      int64  `json:"startedAt,omitempty" firestore:"startedAt"`
	UpdatedAt      int64  `json:"updatedAt,omitempty" firestore:"updatedAt"`
}

type Content struct {
	Title                string                  `json:"title" firestore:"title"`
	Caption              string                  `json:"caption,omitempty" firestore:"caption"`
	Hashtags             string                  `json:"hashtags,omitempty" firestore:"hashtags"`
	Script               string                  `json:"script,omitempty" firestore:"script"`
	Status               string                  `json:"status" firestore:"status"`
	ContentFormat        string                  `json:"contentFormat" firestore:"contentFormat"`
	Attachments          []ContentAttachment     `json:"attachments,omitempty" firestore:"attachments"`
	Destinations         []ContentDestination    `json:"destinations,omitempty" firestore:"destinations"`
	ImageGeneration      *ContentImageGeneration `json:"imageGeneration,omitempty" firestore:"imageGeneration"`
	ScheduleMode         string                  `json:"scheduleMode,omitempty" firestore:"scheduleMode"`
	ScheduledAt          int64                   `json:"scheduledAt,omitempty" firestore:"scheduledAt"`
	ScheduleExecutionArn string                  `json:"scheduleExecutionArn,omitempty" firestore:"scheduleExecutionArn"`
	PublishedIds         map[string]string       `json:"publishedIds,omitempty" firestore:"publishedIds"`
	PublishError         string                  `json:"publishError,omitempty" firestore:"publishError"`
	PostedURL            string                  `json:"postedUrl,omitempty" firestore:"postedUrl"`
}

// GetContent reads a single content document for a brand.
func GetContent(brandID, contentID string) (*Content, error) {
	doc, err := firestoredb.Client.
		Collection(fmt.Sprintf("brands/%s/contents", brandID)).
		Doc(contentID).
		Get(context.Background())
	if err != nil {
		return nil, err
	}
	var ct Content
	if err := doc.DataTo(&ct); err != nil {
		return nil, err
	}
	return &ct, nil
}

// UpdateContentFields merge-updates the given fields on a content document and
// always bumps updatedAt.
func UpdateContentFields(brandID, contentID string, fields map[string]interface{}) error {
	fields["updatedAt"] = time.Now().UnixMilli()
	_, err := firestoredb.Client.
		Collection(fmt.Sprintf("brands/%s/contents", brandID)).
		Doc(contentID).
		Set(context.Background(), fields, firestore.MergeAll)
	return err
}
