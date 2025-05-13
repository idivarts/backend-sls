package trendlymodels

import (
	"context"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

type User struct {
	Name                  string                 `json:"name" firestore:"name"`
	IsChatConnected       bool                   `json:"isChatConnected" firestore:"isChatConnected"`
	PrimarySocial         *string                `json:"primarySocial,omitempty" firestore:"primarySocial,omitempty"`
	ProfileImage          *string                `json:"profileImage,omitempty" firestore:"profileImage,omitempty"`
	Email                 *string                `json:"email,omitempty" firestore:"email,omitempty"`
	PhoneNumber           *string                `json:"phoneNumber,omitempty" firestore:"phoneNumber,omitempty"`
	Location              *string                `json:"location,omitempty" firestore:"location,omitempty"`
	EmailVerified         *bool                  `json:"emailVerified,omitempty" firestore:"emailVerified,omitempty"`
	PhoneVerified         *bool                  `json:"phoneVerified,omitempty" firestore:"phoneVerified,omitempty"`
	IsVerified            *bool                  `json:"isVerified,omitempty" firestore:"isVerified,omitempty"`
	Profile               *UserProfile           `json:"profile,omitempty" firestore:"profile,omitempty"`
	Backend               *BackendData           `json:"backend,omitempty" firestore:"backend,omitempty"`
	PushNotificationToken *PushNotificationToken `json:"pushNotificationToken,omitempty" firestore:"pushNotificationToken,omitempty"`
	Preferences           *UserPreferences       `json:"preferences,omitempty" firestore:"preferences,omitempty"`
	Settings              *UserSettings          `json:"settings,omitempty" firestore:"settings,omitempty"`
	CreationTime          *int64                 `json:"creationTime,omitempty" firestore:"creationTime,omitempty"`
	LastUseTime           *int64                 `json:"lastUseTime,omitempty" firestore:"lastUseTime,omitempty"`
	UpdateTime            *int64                 `json:"updateTime,omitempty" firestore:"updateTime,omitempty"`

	// These are the subcollections to be handled
	// Notifications         []Notification         `json:"notifications" firestore:"notifications"`
	// Socials               []SocialMediaAccount   `json:"socials" firestore:"socials"`
}

type UserProfile struct {
	CompletionPercentage *int                `json:"completionPercentage,omitempty" firestore:"completionPercentage,omitempty"`
	Content              *UserProfileContent `json:"content,omitempty" firestore:"content,omitempty"`
	IntroVideo           *string             `json:"introVideo,omitempty" firestore:"introVideo,omitempty"`
	Category             []string            `json:"category,omitempty" firestore:"category,omitempty"`
	Attachments          []UserAttachment    `json:"attachments,omitempty" firestore:"attachments,omitempty"`
	TimeCommitment       *string             `json:"timeCommitment,omitempty" firestore:"timeCommitment,omitempty"`
}

type UserProfileContent struct {
	About                *string `json:"about,omitempty" firestore:"about,omitempty"`
	SocialMediaHighlight *string `json:"socialMediaHighlight,omitempty" firestore:"socialMediaHighlight,omitempty"`
	CollaborationGoals   *string `json:"collaborationGoals,omitempty" firestore:"collaborationGoals,omitempty"`
	AudienceInsights     *string `json:"audienceInsights,omitempty" firestore:"audienceInsights,omitempty"`
	FunFactAboutUser     *string `json:"funFactAboutUser,omitempty" firestore:"funFactAboutUser,omitempty"`
}

type UserAttachment struct {
	Type     string  `json:"type" firestore:"type"`
	AppleURL *string `json:"appleUrl,omitempty" firestore:"appleUrl,omitempty"`
	PlayURL  *string `json:"playUrl,omitempty" firestore:"playUrl,omitempty"`
	ImageURL *string `json:"imageUrl,omitempty" firestore:"imageUrl,omitempty"`
}

type UserPreferences struct {
	BudgetForPaidCollabs       []int    `json:"budgetForPaidCollabs,omitempty" firestore:"budgetForPaidCollabs,omitempty"`
	ContentCategory            []string `json:"contentCategory,omitempty" firestore:"contentCategory,omitempty"`
	ContentWillingToPost       []string `json:"contentWillingToPost,omitempty" firestore:"contentWillingToPost,omitempty"`
	Goal                       *string  `json:"goal,omitempty" firestore:"goal,omitempty"`
	MaximumMonthlyCollabs      []int    `json:"maximumMonthlyCollabs,omitempty" firestore:"maximumMonthlyCollabs,omitempty"`
	PreferredBrandIndustries   []string `json:"preferredBrandIndustries,omitempty" firestore:"preferredBrandIndustries,omitempty"`
	PreferredCollaborationType *string  `json:"preferredCollaborationType,omitempty" firestore:"preferredCollaborationType,omitempty"`
	PreferredLanguages         []string `json:"preferredLanguages,omitempty" firestore:"preferredLanguages,omitempty"`
	PreferredVideoType         *string  `json:"preferredVideoType,omitempty" firestore:"preferredVideoType,omitempty"`
}

type UserSettings struct {
	AccountStatus     *string `json:"accountStatus,omitempty" firestore:"accountStatus,omitempty"`
	Availability      *string `json:"availability,omitempty" firestore:"availability,omitempty"`
	DataSharing       *string `json:"dataSharing,omitempty" firestore:"dataSharing,omitempty"`
	EmailNotification *bool   `json:"emailNotification,omitempty" firestore:"emailNotification,omitempty"`
	ProfileVisibility *string `json:"profileVisibility,omitempty" firestore:"profileVisibility,omitempty"`
	PushNotification  *bool   `json:"pushNotification,omitempty" firestore:"pushNotification,omitempty"`
	Theme             *string `json:"theme,omitempty" firestore:"theme,omitempty"`
}

type BackendData struct {
	Followers  *int `json:"followers,omitempty" firestore:"followers,omitempty"`
	Reach      *int `json:"reach,omitempty" firestore:"reach,omitempty"`
	Engagement *int `json:"engagement,omitempty" firestore:"engagement,omitempty"`
	Rating     *int `json:"rating,omitempty" firestore:"rating,omitempty"`
}

type SocialMediaAccount struct {
	// Define Social Media Account structure fields here
}

type PushNotificationToken struct {
	IOS     []string `json:"ios,omitempty" firestore:"ios,omitempty"`
	Android []string `json:"android,omitempty" firestore:"android,omitempty"`
	Web     []string `json:"web,omitempty" firestore:"web,omitempty"`
}

func (u *User) Insert(uid string) (*firestore.WriteResult, error) {
	res, err := firestoredb.Client.Collection("users").Doc(uid).Set(context.Background(), u, firestore.MergeAll)

	if err != nil {
		return nil, err
	}
	return res, err
}

func (u *User) Get(uid string) error {
	res, err := firestoredb.Client.Collection("users").Doc(uid).Get((context.Background()))
	if err != nil {
		return err
	}
	err = res.DataTo(u)
	if err != nil {
		return err
	}
	return err
}
