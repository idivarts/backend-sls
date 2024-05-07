package sqsevents

type ConversationEvent struct {
	Action   SQSEvents `json:"action"`
	IGSID    string    `json:"igsid"`
	ThreadID string    `json:"threadId"`
	MID      string    `json:"mid"`
	RunID    string    `json:"runID"`
}
