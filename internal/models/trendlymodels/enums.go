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
