package trendlymodels

import (
	"context"
	"encoding/json"
	"fmt"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"google.golang.org/api/iterator"
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
	IsKYCDone             bool                   `json:"isKYCDone" firestore:"isKYCDone"`
	KYC                   *KYC                   `json:"kyc,omitempty" firestore:"kyc,omitempty"`
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
	BudgetForPaidCollabs []int `json:"budgetForPaidCollabs,omitempty" firestore:"budgetForPaidCollabs,omitempty"`
	// ContentCategory            []string `json:"contentCategory,omitempty" firestore:"contentCategory,omitempty"`
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

type KYC struct {
	AccountID     string `json:"accountId" firestore:"accountId"`
	StakeHolderID string `json:"stakeHolderId" firestore:"stakeHolderId"`
	ProductID     string `json:"productId" firestore:"productId"`

	Status    string  `json:"status" firestore:"status"`
	Reason    *string `json:"reason,omitempty" firestore:"reason,omitempty"`
	UpdatedAt *int64  `json:"updatedAt,omitempty" firestore:"updatedAt,omitempty"`

	PANDetails     *PANDetails     `json:"panDetails,omitempty" firestore:"panDetails,omitempty"`
	CurrentAddress *CurrentAddress `json:"currentAddress,omitempty" firestore:"currentAddress,omitempty"`
	BankDetails    *BankDetails    `json:"bankDetails,omitempty" firestore:"bankDetails,omitempty"`
}

type PANDetails struct {
	PANNumber    string `json:"panNumber" firestore:"panNumber"`
	NameAsPerPAN string `json:"nameAsPerPAN" firestore:"nameAsPerPAN"`
}

type CurrentAddress struct {
	Street     string `json:"street" firestore:"street"`
	City       string `json:"city" firestore:"city"`
	State      string `json:"state" firestore:"state"`
	PostalCode string `json:"postalCode" firestore:"postalCode"`
}

type BankDetails struct {
	AccountNumber   string `json:"accountNumber" firestore:"accountNumber"`
	IFSC            string `json:"ifsc" firestore:"ifsc"`
	BeneficiaryName string `json:"beneficiaryName" firestore:"beneficiaryName"`
}

type BackendData struct {
	Followers  *int64 `json:"followers,omitempty" firestore:"followers,omitempty"`
	Reach      *int64 `json:"reach,omitempty" firestore:"reach,omitempty"`
	Engagement *int64 `json:"engagement,omitempty" firestore:"engagement,omitempty"`
	Rating     *int64 `json:"rating,omitempty" firestore:"rating,omitempty"`

	Gender  *string `json:"gender,omitempty" firestore:"gender,omitempty"`
	Quality *int    `json:"quality,omitempty" firestore:"quality,omitempty"`
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
	// Marshal the struct to JSON
	bytes, err := json.Marshal(u)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal user: %w", err)
	}

	// Unmarshal into a map
	var data map[string]interface{}
	if err := json.Unmarshal(bytes, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal to map: %w", err)
	}

	res, err := firestoredb.Client.Collection("users").Doc(uid).Set(context.Background(), data, firestore.MergeAll)

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

func GetInfluencerIDs(startAfter *interface{}, limit int) ([]string, error) {
	var iter *firestore.DocumentIterator

	collection := firestoredb.Client.Collection("users").Where("profile.completionPercentage", ">=", 60).OrderBy("lastUseTime", firestore.Desc)
	if startAfter == nil {
		iter = collection.Limit(limit).Documents(context.Background())
	} else {
		iter = collection.StartAfter(startAfter).Limit(limit).Documents(context.Background())
	}

	defer iter.Stop()
	influencers := []string{}
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		influencers = append(influencers, doc.Ref.ID)
	}
	return influencers, nil
}
