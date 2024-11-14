package models

type Collectible struct {
	OrganizationID string `json:"organizationId" firestore:"organizationId"`
	CampaignID     string `json:"campaignId" firestore:"campaignId"`
	LeadStageID    string `json:"leadStageId" firestore:"leadStageId"`
	Name           string `json:"name" firestore:"name"`
	Type           string `json:"type" firestore:"type"`
	Description    string `json:"description" firestore:"description"`
	Mandatory      bool   `json:"mandatory" firestore:"mandatory"`
}
