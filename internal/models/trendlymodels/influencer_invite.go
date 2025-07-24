package trendlymodels

import (
	"context"
	"errors"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

type InfluencerInvite struct {
	InfluencerId string   `json:"influencerId" firestore:"influencerId"`
	Category     string   `json:"category" firestore:"category"`
	Reason       string   `json:"reason" firestore:"reason"`
	CollabType   []string `json:"collabType" firestore:"collabType"`
	ExampleLink  string   `json:"exampleLink" firestore:"exampleLink"`
	Platforms    []string `json:"platforms" firestore:"platforms"`
	CollabMode   string   `json:"collabMode" firestore:"collabMode"`
	BudgetMin    *int     `json:"budgetMin,omitempty" firestore:"budgetMin,omitempty"`
	BudgetMax    *int     `json:"budgetMax,omitempty" firestore:"budgetMax,omitempty"`
	Status       int      `json:"status" firestore:"status"` // 0: Pending, 1: Accepted, 2: Rejected
}

func (b *InfluencerInvite) Get(userID, influencerId string) error {
	res, err := firestoredb.Client.Collection("users").Doc(userID).Collection("invitations").Doc(influencerId).Get(context.Background())
	if err != nil {
		return err
	}

	err = res.DataTo(b)
	if err != nil {
		return err
	}
	return err
}

func (s *InfluencerInvite) Insert(userId string) (*firestore.WriteResult, error) {
	if s.InfluencerId == "" {
		return nil, errors.New("influencerId-required")
	}
	res, err := firestoredb.Client.Collection("users").Doc(userId).Collection("invitations").Doc(s.InfluencerId).Set(context.Background(), s)

	if err != nil {
		return nil, err
	}
	return res, err
}
