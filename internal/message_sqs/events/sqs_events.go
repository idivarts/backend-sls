package sqsevents

type ConversationEvent struct {
	Action   SQSEvents `json:"action"`
	IGSID    string    `json:"igsid"`
	ThreadID string    `json:"threadId,omitempty"`
	MID      string    `json:"mid,omitempty"`

	RunID string `json:"runId,omitempty"`

	PageID string `json:"pageId,omitempty"`

	PageToken   string `json:"pageToken,omitempty"`
	Message     string `json:"message,omitempty"`
	LastMessage *bool  `json:"lastMessage,omitempty"`
}
