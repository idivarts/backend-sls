package trendlymodels

type KYCStatus string

const (
	KYCStatusNotStarted         KYCStatus = "not_started"
	KYCStatusInProgress         KYCStatus = "in_progress"
	KYCStatusUnderReview        KYCStatus = "under_review"
	KYCStatusNeedsClarification KYCStatus = "needs_clarification"
	KYCStatusActivated          KYCStatus = "activated"
	KYCStatusRejected           KYCStatus = "rejected"
)

type ContractStatus int

const (
	ContractStatusPending            ContractStatus = 0
	ContractStatusOrderCreated       ContractStatus = 1
	ContractStatusPaymentFailed      ContractStatus = 2
	ContractStatusShipmentPending    ContractStatus = 3
	ContractStatusShipped            ContractStatus = 4
	ContractStatusDelivered          ContractStatus = 5
	ContractStatusDeliverablePending ContractStatus = 6
	ContractStatusDeliverableSent    ContractStatus = 7
	ContractStatusPostScheduled      ContractStatus = 8
	ContractStatusPostDone           ContractStatus = 9
	ContractStatusSettled            ContractStatus = 10
	ContractStatusCancelled          ContractStatus = 11
	ContractStatusDisputed           ContractStatus = 12
)

type PaymentStatus string

const (
	PaymentStatusWaitingForPayment PaymentStatus = "waiting-for-payment"
	PaymentStatusFailed            PaymentStatus = "failed"
	PaymentStatusPaid              PaymentStatus = "paid"
	PaymentStatusTransferProcessed PaymentStatus = "transfer-processed"
	PaymentStatusTransferFailed    PaymentStatus = "transfer-failed"
)

type ShipmentStatus string

const (
	ShipmentStatusShipped   ShipmentStatus = "shipped"
	ShipmentStatusDelivered ShipmentStatus = "delivered"
	ShipmentStatusReceived  ShipmentStatus = "received"
)

type DeliverableStatus string

const (
	DeliverableStatusRevisionRequested DeliverableStatus = "revision-requested"
	DeliverableStatusSubmitted         DeliverableStatus = "submitted"
)

type PostingStatus string

const (
	PostingStatusApproved    PostingStatus = "approved"
	PostingStatusRescheduled PostingStatus = "rescheduled"
	PostingStatusPosted      PostingStatus = "posted"
)

type PostingScenario string

const (
	PostingScenarioInfluencerWillPost          PostingScenario = "influencer-will-post"
	PostingScenarioInfluencerBrandCollabPost   PostingScenario = "influencer-and-brand-collab-post"
	PostingScenarioBrandUsesVideoIndependently PostingScenario = "brand-will-use-video-independently"
)
