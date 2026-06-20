package trendlymodels

import (
	"context"
	"encoding/json"
	"fmt"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"google.golang.org/api/iterator"
)

// InfluencerFeedback is rating and review submitted by the influencer on a contract.
type InfluencerFeedback struct {
	Ratings        *int    `json:"ratings,omitempty" firestore:"ratings,omitempty"`
	FeedbackReview *string `json:"feedbackReview,omitempty" firestore:"feedbackReview,omitempty"`
	TimeSubmitted  *int64  `json:"timeSubmitted,omitempty" firestore:"timeSubmitted,omitempty"`
}

// BrandContractFeedback is rating and review submitted by the brand (manager) on a contract.
type BrandContractFeedback struct {
	Ratings        *int    `json:"ratings,omitempty" firestore:"ratings,omitempty"`
	FeedbackReview *string `json:"feedbackReview,omitempty" firestore:"feedbackReview,omitempty"`
	ManagerID      *string `json:"managerId,omitempty" firestore:"managerId,omitempty"`
	TimeSubmitted  *int64  `json:"timeSubmitted,omitempty" firestore:"timeSubmitted,omitempty"`
}

type Contract struct {
	UserID          string         `json:"userId" firestore:"userId"`
	ManagerID       string         `json:"managerId" firestore:"managerId"`
	CollaborationID string         `json:"collaborationId" firestore:"collaborationId"`
	BrandID         string         `json:"brandId" firestore:"brandId"`
	StreamChannelID string         `json:"streamChannelId" firestore:"streamChannelId"`
	Status          ContractStatus `json:"status" firestore:"status"`

	FeedbackFromBrand *BrandContractFeedback `json:"feedbackFromBrand,omitempty" firestore:"feedbackFromBrand,omitempty"`

	FeedbackFromInfluencer *InfluencerFeedback `json:"feedbackFromInfluencer,omitempty" firestore:"feedbackFromInfluencer,omitempty"`

	ContractTimestamp ContractTimestamp `json:"contractTimestamp" firestore:"contractTimestamp"`

	// All Items for storing the monetization related data
	Payment *Payment `json:"payment,omitempty" firestore:"payment,omitempty"`

	Shipment *Shipment `json:"shipment,omitempty" firestore:"shipment,omitempty"`

	Deliverable *Deliverable `json:"deliverable,omitempty" firestore:"deliverable,omitempty"`

	Posting *Posting `json:"posting,omitempty" firestore:"posting,omitempty"`

	Analytics *Analytics `json:"analytics,omitempty" firestore:"analytics,omitempty"`

	Activity []Activity `json:"activity,omitempty" firestore:"activity,omitempty"`

	Dispute         *DisputeDetails      `json:"dispute,omitempty" firestore:"dispute,omitempty"`
	CancellationReq *CancellationRequest `json:"cancellationRequest,omitempty" firestore:"cancellationRequest,omitempty"`
	SLAWarnings     []SLAWarning         `json:"slaWarnings,omitempty" firestore:"slaWarnings,omitempty"`
}

// DisputeDetails holds the details of a dispute raised on a contract.
type DisputeDetails struct {
	RaisedBy     string   `json:"raisedBy,omitempty" firestore:"raisedBy,omitempty"`
	RaisedByRole string   `json:"raisedByRole,omitempty" firestore:"raisedByRole,omitempty"` // "influencer" | "brand"
	Type         string   `json:"type,omitempty" firestore:"type,omitempty"`
	Description  string   `json:"description,omitempty" firestore:"description,omitempty"`
	Evidence     []string `json:"evidence,omitempty" firestore:"evidence,omitempty"` // S3 URLs
	Status       string   `json:"status,omitempty" firestore:"status,omitempty"`     // "open" | "under_review" | "resolved" | "closed"
	RaisedAt     int64    `json:"raisedAt,omitempty" firestore:"raisedAt,omitempty"`
	ResolvedAt   int64    `json:"resolvedAt,omitempty" firestore:"resolvedAt,omitempty"`
	Resolution   string   `json:"resolution,omitempty" firestore:"resolution,omitempty"`
	AdminID      string   `json:"adminId,omitempty" firestore:"adminId,omitempty"`
}

// CancellationRequest tracks a pending contract cancellation request.
type CancellationRequest struct {
	RequestedBy     string `json:"requestedBy,omitempty" firestore:"requestedBy,omitempty"`
	RequestedByRole string `json:"requestedByRole,omitempty" firestore:"requestedByRole,omitempty"` // "influencer" | "brand"
	Reason          string `json:"reason,omitempty" firestore:"reason,omitempty"`
	Status          string `json:"status,omitempty" firestore:"status,omitempty"` // "pending" | "approved" | "rejected"
	RequestedAt     int64  `json:"requestedAt,omitempty" firestore:"requestedAt,omitempty"`
	RespondedAt     int64  `json:"respondedAt,omitempty" firestore:"respondedAt,omitempty"`
	RefundAmount    int64  `json:"refundAmount,omitempty" firestore:"refundAmount,omitempty"` // paise; 0 for barter/no-refund
}

// SLAWarning records an SLA nudge or escalation sent for a contract.
type SLAWarning struct {
	Type   string `json:"type,omitempty" firestore:"type,omitempty"`   // e.g. "shipment_overdue"
	Level  string `json:"level,omitempty" firestore:"level,omitempty"` // "nudge" | "support_escalation"
	SentAt int64  `json:"sentAt,omitempty" firestore:"sentAt,omitempty"`
}

type ContractTimestamp struct {
	StartedOn int64  `json:"startedOn" firestore:"startedOn"`
	EndedOn   *int64 `json:"endedOn,omitempty" firestore:"endedOn,omitempty"`
}

type Payment struct {
	OrderID    string        `json:"orderId,omitempty" firestore:"orderId,omitempty"`
	Status     PaymentStatus `json:"status,omitempty" firestore:"status,omitempty"`
	PaymentID  string        `json:"paymentId,omitempty" firestore:"paymentId,omitempty"`
	TransferID string        `json:"transferId,omitempty" firestore:"transferId,omitempty"`
	ShortURL   string        `json:"shortUrl,omitempty" firestore:"shortUrl,omitempty"`
	Amount     int           `json:"amount,omitempty" firestore:"amount,omitempty"`
}

type Shipment struct {
	TrackingID         string         `json:"trackingId,omitempty" firestore:"trackingId,omitempty"`
	ShipmentProvider   string         `json:"shipmentProvider,omitempty" firestore:"shipmentProvider,omitempty"`
	ExpectedDate       int64          `json:"expectedDate,omitempty" firestore:"expectedDate,omitempty"`
	PackageScreenshots []string       `json:"packageScreenshots,omitempty" firestore:"packageScreenshots,omitempty"`
	AddressShippedTo   interface{}    `json:"addressShippedTo,omitempty" firestore:"addressShippedTo,omitempty"`
	Status             ShipmentStatus `json:"status,omitempty" firestore:"status,omitempty"`
	Notes              string         `json:"notes,omitempty" firestore:"notes,omitempty"`
	ReceivedNotes      string         `json:"receivedNotes,omitempty" firestore:"receivedNotes,omitempty"`
}
type Deliverable struct {
	Status           DeliverableStatus `json:"status,omitempty" firestore:"status,omitempty"`
	DeliverableLinks []string          `json:"deliverableLinks,omitempty" firestore:"deliverableLinks,omitempty"`
	Notes            string            `json:"notes,omitempty" firestore:"notes,omitempty"`
	RevisionCount    int               `json:"revisionCount,omitempty" firestore:"revisionCount,omitempty"`
	RevisionNotes    []string          `json:"revisionNotes,omitempty" firestore:"revisionNotes,omitempty"`
}
type Posting struct {
	ScheduledDate   int64           `json:"scheduledDate,omitempty" firestore:"scheduledDate,omitempty"`
	Status          PostingStatus   `json:"status,omitempty" firestore:"status,omitempty"`
	PostedLinks     []string        `json:"postedLinks,omitempty" firestore:"postedLinks,omitempty"`
	PostingScenario PostingScenario `json:"postingScenario,omitempty" firestore:"postingScenario,omitempty"`
	ProofScreenshot string          `json:"proofScreenshot,omitempty" firestore:"proofScreenshot,omitempty"`
	PostURL         string          `json:"postUrl,omitempty" firestore:"postUrl,omitempty"`
	Notes           string          `json:"notes,omitempty" firestore:"notes,omitempty"`
}
type Analytics struct {
	Views       int `json:"views,omitempty" firestore:"views,omitempty"`
	Likes       int `json:"likes,omitempty" firestore:"likes,omitempty"`
	Comments    int `json:"comments,omitempty" firestore:"comments,omitempty"`
	Shares      int `json:"shares,omitempty" firestore:"shares,omitempty"`
	Impressions int `json:"impressions,omitempty" firestore:"impressions,omitempty"`
}
type Activity struct {
	Type    string      `json:"type,omitempty" firestore:"type,omitempty"`
	Time    int64       `json:"time,omitempty" firestore:"time,omitempty"`
	Detail  string      `json:"detail,omitempty" firestore:"detail,omitempty"`
	Payload interface{} `json:"payload,omitempty" firestore:"payload,omitempty"`
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

func (c *Contract) Update(contractID string) error {
	// Marshal the struct to JSON
	bytes, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal user: %w", err)
	}

	// Unmarshal into a map
	var data map[string]interface{}
	if err := json.Unmarshal(bytes, &data); err != nil {
		return fmt.Errorf("failed to unmarshal to map: %w", err)
	}
	_, err = firestoredb.Client.Collection("contracts").Doc(contractID).Set(context.Background(), data, firestore.MergeAll)

	return err
}

// HasActiveContracts reports whether the brand has any contract that is not in a
// terminal state (Settled or Cancelled). Used as a guard before deleting a
// brand so in-flight collaborations/payouts are never orphaned.
func HasActiveContracts(brandID string) (bool, error) {
	iter := firestoredb.Client.Collection("contracts").Where("brandId", "==", brandID).Documents(context.Background())
	defer iter.Stop()

	for {
		doc, err := iter.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			return false, err
		}
		var c Contract
		if err := doc.DataTo(&c); err != nil {
			return false, err
		}
		if c.Status != ContractStatusSettled && c.Status != ContractStatusCancelled {
			return true, nil
		}
	}
	return false, nil
}

func (b *Contract) GetByCollab(collabId, userId string) error {
	iter := firestoredb.Client.Collection("contracts").Where("collaborationId", "==", collabId).Where("userId", "==", userId).Limit(1).Documents(context.Background())

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
