package trendlymodels

import (
	"context"

	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

type Contract struct {
	UserID          string `json:"userId" firestore:"userId"`
	ManagerID       string `json:"managerId" firestore:"managerId"`
	CollaborationID string `json:"collaborationId" firestore:"collaborationId"`
	BrandID         string `json:"brandId" firestore:"brandId"`
	StreamChannelID string `json:"streamChannelId" firestore:"streamChannelId"`
	Status          int    `json:"status" firestore:"status"`

	// All Items for storing the monetization related data
	Payment struct {
		OrderID         string   `json:"orderId,omitempty" firestore:"orderId,omitempty"`
		Status          string   `json:"status,omitempty" firestore:"status,omitempty"`
		PaymentID       string   `json:"paymentId,omitempty" firestore:"paymentId,omitempty"`
		PaymentWebhooks []string `json:"paymentWebhooks,omitempty" firestore:"paymentWebhooks,omitempty"`
	} `json:"payment" firestore:"payment"`

	Shipment struct {
		ShipmentID      string   `json:"shipmentId,omitempty" firestore:"shipmentId,omitempty"`
		Status          string   `json:"status,omitempty" firestore:"status,omitempty"`
		ShipmentUpdates []string `json:"shipmentUpdates,omitempty" firestore:"shipmentUpdates,omitempty"`
	} `json:"shipment" firestore:"shipment"`

	Deliverable struct {
		DeliverableID    string   `json:"deliverableId,omitempty" firestore:"deliverableId,omitempty"`
		Status           string   `json:"status,omitempty" firestore:"status,omitempty"`
		DeliverableLinks []string `json:"deliverableLinks,omitempty" firestore:"deliverableLinks,omitempty"`
		ApprovalRequests []string `json:"approvalRequests,omitempty" firestore:"approvalRequests,omitempty"`
	} `json:"deliverable" firestore:"deliverable"`

	Posting struct {
		ScheduledDate      int64    `json:"scheduledDate,omitempty" firestore:"scheduledDate,omitempty"`
		Status             string   `json:"status,omitempty" firestore:"status,omitempty"`
		RescheduleRequests []string `json:"rescheduleRequests,omitempty" firestore:"rescheduleRequests,omitempty"`
		PostedLinks        []string `json:"postedLinks,omitempty" firestore:"postedLinks,omitempty"`
	} `json:"posting" firestore:"posting"`

	Analytics struct {
		Views       int `json:"views,omitempty" firestore:"views,omitempty"`
		Likes       int `json:"likes,omitempty" firestore:"likes,omitempty"`
		Comments    int `json:"comments,omitempty" firestore:"comments,omitempty"`
		Shares      int `json:"shares,omitempty" firestore:"shares,omitempty"`
		Impressions int `json:"impressions,omitempty" firestore:"impressions,omitempty"`
	} `json:"analytics" firestore:"analytics"`
}

func (b *Contract) Get(contractID string) error {
	res, err := firestoredb.Client.Collection("contracts").Doc(contractID).Get(context.Background())
	if err != nil {
		return err
	}

	err = res.DataTo(b)
	if err != nil {
		return err
	}
	return err
}

func (b *Contract) GetByCollab(collabId, userId string) error {
	iter := firestoredb.Client.Collection("contracts").Where("collaborationId", "==", collabId).Where("userId", "==", userId).Documents(context.Background())

	res, err := iter.Next()
	if err != nil {
		return err
	}

	err = res.DataTo(b)
	if err != nil {
		return err
	}
	return err
}
