package trendlymodels

import (
	"context"
	"encoding/json"
	"fmt"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"google.golang.org/api/iterator"
)

type Collaboration struct {
	Name                      string                   `firestore:"name" json:"name"`
	BrandID                   string                   `firestore:"brandId" json:"brandId"`
	ManagerID                 string                   `firestore:"managerId" json:"managerId"`
	Attachments               []interface{}            `firestore:"attachments,omitempty" json:"attachments,omitempty"`
	Description               string                   `firestore:"description,omitempty" json:"description,omitempty"`
	PromotionType             string                   `firestore:"promotionType" json:"promotionType"`
	Budget                    *Budget                  `firestore:"budget,omitempty" json:"budget,omitempty"`
	PreferredContentLanguage  []string                 `firestore:"preferredContentLanguage" json:"preferredContentLanguage"`
	ContentFormat             []string                 `firestore:"contentFormat" json:"contentFormat"`
	Platform                  []string                 `firestore:"platform" json:"platform"`
	NumberOfInfluencersNeeded int                      `firestore:"numberOfInfluencersNeeded" json:"numberOfInfluencersNeeded"`
	Location                  CollaborationLocation    `firestore:"location" json:"location"`
	ExternalLinks             []interface{}            `firestore:"externalLinks,omitempty" json:"externalLinks,omitempty"`
	QuestionsToInfluencers    []string                 `firestore:"questionsToInfluencers,omitempty" json:"questionsToInfluencers,omitempty"`
	Preferences               CollaborationPreferences `firestore:"preferences" json:"preferences"`
	Status                    string                   `firestore:"status" json:"status"`
	Applications              interface{}              `firestore:"applications" json:"applications"`
	Invitations               interface{}              `firestore:"invitations" json:"invitations"`
	TimeStamp                 int64                    `firestore:"timeStamp" json:"timeStamp"`
	ViewsLastHour             *int                     `firestore:"viewsLastHour,omitempty" json:"viewsLastHour,omitempty"`
	LastReviewedTimeStamp     *int64                   `firestore:"lastReviewedTimeStamp,omitempty" json:"lastReviewedTimeStamp,omitempty"`
}

type Budget struct {
	Min *int `firestore:"min,omitempty" json:"min,omitempty"`
	Max *int `firestore:"max,omitempty" json:"max,omitempty"`
}

type CollaborationLocation struct {
	Type    string   `firestore:"type" json:"type"`
	Name    string   `firestore:"name,omitempty" json:"name,omitempty"`
	LatLong *LatLong `firestore:"latlong,omitempty" json:"latlong,omitempty"`
}

type LatLong struct {
	Lat  float64 `firestore:"lat" json:"lat"`
	Long float64 `firestore:"long" json:"long"`
}

type CollaborationPreferences struct {
	TimeCommitment     string   `firestore:"timeCommitment" json:"timeCommitment"`
	InfluencerNiche    []string `firestore:"influencerNiche" json:"influencerNiche"`
	InfluencerRelation string   `firestore:"influencerRelation" json:"influencerRelation"`
	PreferredVideoType string   `firestore:"preferredVideoType" json:"preferredVideoType"`
}

func (b *Collaboration) Get(collabId string) error {
	res, err := firestoredb.Client.Collection("collaborations").Doc(collabId).Get(context.Background())
	if err != nil {
		return err
	}

	err = res.DataTo(b)
	if err != nil {
		return err
	}
	return err
}

func (b *Collaboration) Insert(collabId string) (*firestore.WriteResult, error) {
	bytes, err := json.Marshal(b)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal user: %w", err)
	}

	// Unmarshal into a map
	var data map[string]interface{}
	if err := json.Unmarshal(bytes, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal to map: %w", err)
	}

	res, err := firestoredb.Client.Collection("collaborations").Doc(collabId).Set(context.Background(), data, firestore.MergeAll)
	if err != nil {
		return nil, err
	}

	return res, err
}

func GetCollabIDs(startAfter *interface{}, limit int) ([]string, error) {
	var iter *firestore.DocumentIterator

	collection := firestoredb.Client.Collection("collaborations").Where("status", "in", []string{"active", "stopped"}).OrderBy("timeStamp", firestore.Desc)
	if startAfter == nil {
		iter = collection.Limit(limit).Documents(context.Background())
	} else {
		iter = collection.StartAfter(startAfter).Limit(limit).Documents(context.Background())
	}

	defer iter.Stop()

	collabs := []string{}
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		collabs = append(collabs, doc.Ref.ID)
	}
	return collabs, nil
}
