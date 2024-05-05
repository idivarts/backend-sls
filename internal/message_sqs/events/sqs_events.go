package sqsevents

type ConversationEvent struct {
	Action   string `json:"action"`
	IGSID    string `json:"igsid"`
	ThreadID string `json:"threadId"`
	MID      string `json:"mid"`
}
