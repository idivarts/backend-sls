package trendlyCollabs

// UserFeedbackRequest is the JSON body for POST /contracts/:contractId/user-feedback (influencer rates the brand).
type UserFeedbackRequest struct {
	Ratings        int    `json:"ratings" binding:"required,gte=1,lte=5"`
	FeedbackReview string `json:"feedbackReview,omitempty"`
}

// BrandFeedbackRequest is the JSON body for POST /contracts/:contractId/brand-feedback (brand rates the influencer).
type BrandFeedbackRequest struct {
	Ratings        int    `json:"ratings" binding:"required,gte=1,lte=5"`
	FeedbackReview string `json:"feedbackReview,omitempty"`
}
