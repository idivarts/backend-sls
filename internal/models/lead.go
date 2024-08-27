package models

type UserProfile struct {
	Name                 string `json:"name"`
	Username             string `json:"username"`
	ProfilePic           string `json:"profile_pic"`
	FollowerCount        int    `json:"follower_count"`
	IsUserFollowBusiness bool   `json:"is_user_follow_business"`
	IsBusinessFollowUser bool   `json:"is_business_follow_user"`
}

type ILeads struct {
	IGSID       *string      `json:"igsid,omitempty"`
	FbID        *string      `json:"fbid,omitempty"`
	Email       *string      `json:"email,omitempty"`
	Name        *string      `json:"name,omitempty"`
	SourceType  SourceType   `json:"sourceType"`
	SourceID    string       `json:"sourceId"`
	UserProfile *UserProfile `json:"userProfile,omitempty"`
	TagID       *string      `json:"tagId,omitempty"`
	CampaignID  *string      `json:"campaignId,omitempty"`
	Status      int          `json:"status"`
	CreatedAt   int64        `json:"createdAt"`
	UpdatedAt   int64        `json:"updatedAt"`
}
