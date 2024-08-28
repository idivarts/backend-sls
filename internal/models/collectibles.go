package models

type Collectible struct {
	OrganizationID string `json:"organizationId"`
	CampaignID     string `json:"campaignId"`
	LeadStageID    string `json:"leadStageId"`
	Name           string `json:"name"`
	Type           string `json:"type"`
	Description    string `json:"description"`
	Mandatory      bool   `json:"mandatory"`
}
