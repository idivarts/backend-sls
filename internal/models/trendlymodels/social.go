package trendlymodels

import "github.com/idivarts/backend-sls/pkg/messenger"

type Socials struct {
	ID           string                      `json:"id" firestore:"id"`
	Name         string                      `json:"name" firestore:"name"`
	Image        string                      `json:"image" firestore:"image"`
	IsInstagram  bool                        `json:"isInstagram" firestore:"isInstagram"`
	ConnectedId  *string                     `json:"connectedId,omitempty" firestore:"connectedId"`
	InstaProfile *messenger.InstagramProfile `json:"instaProfile,omitempty" firestore:"instaProfile"`
	FBProfile    *messenger.FacebookProfile  `json:"fbProfile,omitempty" firestore:"fbProfile"`
}

type SocialsPrivate struct {
	AccessToken *string `json:"accessToken,omitempty"`
}
