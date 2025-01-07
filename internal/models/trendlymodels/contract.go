package trendlymodels

type Contract struct {
	UserID          string `json:"userId" firestore:"userId"`
	ManagerID       string `json:"managerId" firestore:"managerId"`
	CollaborationID string `json:"collaborationId" firestore:"collaborationId"`
	BrandID         string `json:"brandId" firestore:"brandId"`
	StreamChannelID string `json:"streamChannelId" firestore:"streamChannelId"`
	Status          int    `json:"status" firestore:"status"`
}
