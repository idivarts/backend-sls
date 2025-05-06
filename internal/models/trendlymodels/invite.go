package trendlymodels

type Invitation struct {
	UserID          string `json:"userId" firestore:"userId"`
	CollaborationID string `json:"collaborationId" firestore:"collaborationId"`
	ManagerID       string `json:"managerId" firestore:"managerId"`
	Status          string `json:"status" firestore:"status"`
	TimeStamp       int64  `json:"timeStamp" firestore:"timeStamp"`
	Message         string `json:"message" firestore:"message"`
}
