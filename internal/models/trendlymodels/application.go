package trendlymodels

type Application struct {
	UserID                string                      `json:"userId" firestore:"userId"`
	CollaborationID       string                      `json:"collaborationId" firestore:"collaborationId"`
	Status                string                      `json:"status" firestore:"status"`
	TimeStamp             int64                       `json:"timeStamp" firestore:"timeStamp"`
	Message               string                      `json:"message" firestore:"message"`
	Quotation             string                      `json:"quotation" firestore:"quotation"`
	AnswersFromInfluencer []InfluencerAnswer          `json:"answersFromInfluencer" firestore:"answersFromInfluencer"`
	Timeline              int64                       `json:"timeline" firestore:"timeline"`
	Attachments           []interface{}               `json:"attachments" firestore:"attachments"`
	FileAttachments       []ApplicationFileAttachment `json:"fileAttachments" firestore:"fileAttachments"`
}

type InfluencerAnswer struct {
	Question int    `json:"question" firestore:"question"`
	Answer   string `json:"answer" firestore:"answer"`
}

type ApplicationFileAttachment struct {
	URL  string `json:"url" firestore:"url"`
	Name string `json:"name" firestore:"name"`
	Type string `json:"type" firestore:"type"`
}
