package trendlymodels

type Match struct {
	ID        string `json:"id" firestore:"id"`
	IsManager bool   `json:"isManager" firestore:"isManager"`

	Influencer    InfluencerStats
	Brand         BrandPrefs
	AlreadyViewed []ViewStat
}

type InfluencerStats struct {
}
type BrandPrefs struct {
}

type ViewStat struct {
	ID   string
	Time int64
}

// type UserPreferences struct {
// 	BudgetForPaidCollabs []int `json:"budgetForPaidCollabs,omitempty" firestore:"budgetForPaidCollabs,omitempty"`
// 	// ContentCategory            []string `json:"contentCategory,omitempty" firestore:"contentCategory,omitempty"`
// 	ContentWillingToPost       []string `json:"contentWillingToPost,omitempty" firestore:"contentWillingToPost,omitempty"`
// 	Goal                       *string  `json:"goal,omitempty" firestore:"goal,omitempty"`
// 	MaximumMonthlyCollabs      []int    `json:"maximumMonthlyCollabs,omitempty" firestore:"maximumMonthlyCollabs,omitempty"`
// 	PreferredBrandIndustries   []string `json:"preferredBrandIndustries,omitempty" firestore:"preferredBrandIndustries,omitempty"`
// 	PreferredCollaborationType *string  `json:"preferredCollaborationType,omitempty" firestore:"preferredCollaborationType,omitempty"`
// 	PreferredLanguages         []string `json:"preferredLanguages,omitempty" firestore:"preferredLanguages,omitempty"`
// 	PreferredVideoType         *string  `json:"preferredVideoType,omitempty" firestore:"preferredVideoType,omitempty"`
// }

// type BrandPreferences struct {
// 	PromotionType          []string `json:"promotionType,omitempty" firestore:"promotionType,omitempty"`
// 	InfluencerCategories   []string `json:"influencerCategories,omitempty" firestore:"influencerCategories,omitempty"`
// 	Languages              []string `json:"languages,omitempty" firestore:"languages,omitempty"`
// 	Locations              []string `json:"locations,omitempty" firestore:"locations,omitempty"`
// 	Platforms              []string `json:"platforms,omitempty" firestore:"platforms,omitempty"`
// 	CollaborationPostTypes []string `json:"collaborationPostTypes,omitempty" firestore:"collaborationPostTypes,omitempty"`
// 	TimeCommitments        []string `json:"timeCommitments,omitempty" firestore:"timeCommitments,omitempty"`
// 	ContentVideoType       []string `json:"contentVideoType,omitempty" firestore:"contentVideoType,omitempty"`
// }
