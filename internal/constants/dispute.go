package constants

// Dispute types — raised by either party on a contract.
const (
	DisputeShipmentNotReceived  = "shipment_not_received"   // influencer says product not received
	DisputeShipmentDamaged      = "shipment_damaged"        // product received damaged or wrong
	DisputeDeliverableNotSent   = "deliverable_not_sent"    // brand raises: influencer not delivering
	DisputeRevisionAbuse        = "revision_abuse"          // influencer raises: brand requesting excessive revisions
	DisputePostingDefault       = "posting_default"         // brand raises: influencer didn't post or deleted post
	DisputeTermsViolation       = "terms_violation"         // either party: not sticking to agreed terms
	DisputePaymentNotReceived   = "payment_not_received"    // influencer: payout not received
	DisputeOther                = "other"

	// Dispute status values
	DisputeStatusOpen        = "open"
	DisputeStatusUnderReview = "under_review"
	DisputeStatusResolved    = "resolved"
	DisputeStatusClosed      = "closed"

	// Cancellation request status values
	CancellationStatusPending  = "pending"
	CancellationStatusApproved = "approved"
	CancellationStatusRejected = "rejected"

	// SLA warning levels
	SLALevelNudge              = "nudge"
	SLALevelSupportEscalation  = "support_escalation"

	// SLA warning types
	SLAShipmentOverdue             = "shipment_overdue"
	SLAShipmentInTransitTooLong    = "shipment_in_transit_too_long"
	SLADeliveryAckOverdue          = "delivery_ack_overdue"
	SLAVideoOverdue                = "video_overdue"
	SLAReviewOverdue               = "review_overdue"
	SLAPostingOverdue              = "posting_overdue"

	// SLA thresholds (days)
	SLAShipmentNudgeDays          = 5
	SLAShipmentEscalateDays       = 10
	SLAInTransitEscalateDays      = 20
	SLADeliveryAckNudgeDays       = 7
	SLADeliveryAckEscalateDays    = 14
	SLAVideoNudgeDays             = 10
	SLAVideoEscalateDays          = 21
	SLAReviewNudgeDays            = 7
	SLAReviewEscalateDays         = 14
	SLAPostingOverdueDays         = 3
	SLAPostingEscalateDays        = 7

	// Default max revisions when collaboration doesn't specify
	DefaultMaxRevisions = 3
)
