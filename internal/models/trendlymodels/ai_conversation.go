package trendlymodels

type AIConversation struct {
	ID           string `json:"id,omitempty" firestore:"-"`
	BrandID      string `json:"brandId" firestore:"brandId"`
	UserID       string `json:"userId" firestore:"userId"`
	Module       string `json:"module" firestore:"module"`
	ContextID    string `json:"contextId,omitempty" firestore:"contextId,omitempty"`
	Title        string `json:"title" firestore:"title"`
	CurrentModel string `json:"currentModel" firestore:"currentModel"`
	CreatedAt    int64  `json:"createdAt" firestore:"createdAt"`
	UpdatedAt    int64  `json:"updatedAt" firestore:"updatedAt"`
}

type AIMessage struct {
	Role        string `json:"role" firestore:"role"`
	Content     string `json:"content" firestore:"content"`
	Model       string `json:"model,omitempty" firestore:"model,omitempty"`
	FocusedText string `json:"focusedText,omitempty" firestore:"focusedText,omitempty"`
	ImageURL    string `json:"imageUrl,omitempty" firestore:"imageUrl,omitempty"`
	TokenCount  int    `json:"tokenCount,omitempty" firestore:"tokenCount,omitempty"`
	Timestamp   int64  `json:"timestamp" firestore:"timestamp"`
}
