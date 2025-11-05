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
	Name                      string                `firestore:"name" json:"name"`
	BrandID                   string                `firestore:"brandId" json:"brandId"`
	ManagerID                 string                `firestore:"managerId" json:"managerId"`
	Attachments               []interface{}         `firestore:"attachments,omitempty" json:"attachments,omitempty"`
	Description               string                `firestore:"description,omitempty" json:"description,omitempty"`
	PromotionType             string                `firestore:"promotionType" json:"promotionType"`
	Budget                    *Budget               `firestore:"budget,omitempty" json:"budget,omitempty"`
	PreferredContentLanguage  []string              `firestore:"preferredContentLanguage" json:"preferredContentLanguage"`
	ContentFormat             []string              `firestore:"contentFormat" json:"contentFormat"`
	Platform                  []string              `firestore:"platform" json:"platform"`
	NumberOfInfluencersNeeded int                   `firestore:"numberOfInfluencersNeeded" json:"numberOfInfluencersNeeded"`
	Location                  CollaborationLocation `firestore:"location" json:"location"`
	ExternalLinks             []interface{}         `firestore:"externalLinks,omitempty" json:"externalLinks,omitempty"`
	QuestionsToInfluencers    []string              `firestore:"questionsToInfluencers,omitempty" json:"questionsToInfluencers,omitempty"`
	Preferences               *DiscoverPreferences  `firestore:"preferences,omitempty" json:"preferences,omitempty"`
	Status                    string                `firestore:"status" json:"status"`
	Applications              interface{}           `firestore:"applications" json:"applications"`
	Invitations               interface{}           `firestore:"invitations" json:"invitations"`
	TimeStamp                 int64                 `firestore:"timeStamp" json:"timeStamp"`
	ViewsLastHour             *int                  `firestore:"viewsLastHour,omitempty" json:"viewsLastHour,omitempty"`
	LastReviewedTimeStamp     *int64                `firestore:"lastReviewedTimeStamp,omitempty" json:"lastReviewedTimeStamp,omitempty"`
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

type DiscoverPreferences struct {
	// Followers range
	FollowerMin *int64 `firestore:"followerMin,omitempty" json:"followerMin,omitempty"` // minimum followers
	FollowerMax *int64 `firestore:"followerMax,omitempty" json:"followerMax,omitempty"` // maximum followers

	// Content/posts count range
	ContentMin *int `firestore:"contentMin,omitempty" json:"contentMin,omitempty"` // minimum content/posts count
	ContentMax *int `firestore:"contentMax,omitempty" json:"contentMax,omitempty"` // maximum content/posts count

	// Estimated monthly views range
	MonthlyViewMin *int64 `firestore:"monthlyViewMin,omitempty" json:"monthlyViewMin,omitempty"`
	MonthlyViewMax *int64 `firestore:"monthlyViewMax,omitempty" json:"monthlyViewMax,omitempty"`

	// Estimated monthly engagements (likes+comments etc) range
	MonthlyEngagementMin *int64 `firestore:"monthlyEngagementMin,omitempty" json:"monthlyEngagementMin,omitempty"`
	MonthlyEngagementMax *int64 `firestore:"monthlyEngagementMax,omitempty" json:"monthlyEngagementMax,omitempty"`

	// Median/average metrics ranges (counts)
	AvgViewsMin    *int64 `firestore:"avgViewsMin,omitempty" json:"avgViewsMin,omitempty"`
	AvgViewsMax    *int64 `firestore:"avgViewsMax,omitempty" json:"avgViewsMax,omitempty"`
	AvgLikesMin    *int64 `firestore:"avgLikesMin,omitempty" json:"avgLikesMin,omitempty"`
	AvgLikesMax    *int64 `firestore:"avgLikesMax,omitempty" json:"avgLikesMax,omitempty"`
	AvgCommentsMin *int64 `firestore:"avgCommentsMin,omitempty" json:"avgCommentsMin,omitempty"`
	AvgCommentsMax *int64 `firestore:"avgCommentsMax,omitempty" json:"avgCommentsMax,omitempty"`

	// Quality/aesthetics slider (0..100)
	QualityMin *int `firestore:"qualityMin,omitempty" json:"qualityMin,omitempty"`
	QualityMax *int `firestore:"qualityMax,omitempty" json:"qualityMax,omitempty"`

	// Engagement rate (%)
	ERMin *float64 `firestore:"erMin,omitempty" json:"erMin,omitempty"` // e.g., 1.5 => 1.5%
	ERMax *float64 `firestore:"erMax,omitempty" json:"erMax,omitempty"`

	// Text filters
	DescKeywords []string `firestore:"descKeywords,omitempty" json:"descKeywords,omitempty"` // bio keywords (split client-side or server-side)
	Name         *string  `firestore:"name,omitempty" json:"name,omitempty"`

	// Flags
	IsVerified *bool `firestore:"isVerified,omitempty" json:"isVerified,omitempty"`
	HasContact *bool `firestore:"hasContact,omitempty" json:"hasContact,omitempty"`

	// Multi-selects
	Genders           []string `firestore:"genders,omitempty" json:"genders,omitempty"`
	SelectedNiches    []string `firestore:"selectedNiches,omitempty" json:"selectedNiches,omitempty"`
	SelectedLocations []string `firestore:"selectedLocations,omitempty" json:"selectedLocations,omitempty"`

	// Sorting & pagination
	Sort          string `firestore:"sort,omitempty" json:"sort,omitempty"`                     // followers | views | engagement | engagement_rate
	SortDirection string `firestore:"sort_direction,omitempty" json:"sort_direction,omitempty"` // asc | desc (default: desc)
	Offset        *int   `firestore:"offset,omitempty" json:"offset,omitempty"`
	Limit         *int   `firestore:"limit,omitempty" json:"limit,omitempty"`
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
