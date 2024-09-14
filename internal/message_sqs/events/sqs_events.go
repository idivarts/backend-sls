package sqsevents

type ConversationEvent struct {
	Action   SQSEvents `json:"action"`
	LeadID   string    `json:"leadId"`
	ThreadID string    `json:"threadId,omitempty"`
	MID      string    `json:"mid,omitempty"`

	RunID string `json:"runId,omitempty"`

	SourceID string `json:"sourceId,omitempty"`

	PageToken   string `json:"pageToken,omitempty"`
	Message     string `json:"message,omitempty"`
	LastMessage *bool  `json:"lastMessage,omitempty"`
}
