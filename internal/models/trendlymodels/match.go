package trendlymodels

type BQInfluencers struct {
	ID string `json:"id" bigquery:"id"`

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

// -------- USER LISTING
// InfluencerCategories | Category (MATCH)
// PreferredLanguages | Languages (MATCH)
// PreferredBrandIndustries | Industries (MATCH)
// Reach (Sorting)
// Followers (Sorting)
// CompletionPercentage (Sorting)

// -------- COLLAB LISTING

// -------- END

// type UserPreferences struct {
// 	BudgetForPaidCollabs
// 	ContentWillingToPost
// 	Goal
// 	MaximumMonthlyCollabs
// 	PreferredBrandIndustries
// 	PreferredCollaborationType
// 	PreferredLanguages
// 	PreferredVideoType
// }

// type BrandPreferences struct {
// 	PromotionType
// 	InfluencerCategories
// 	Languages
// 	Locations
// 	Platforms
// 	CollaborationPostTypes
// 	TimeCommitments
// 	ContentVideoType
// }
