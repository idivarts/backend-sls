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
	Role string `json:"role" firestore:"role"`
	// UserID/BrandID are denormalized onto every message doc so the Firestore
	// security rules can authorize a client read without a get() on the parent
	// conversation. Stamped by AppendMessage (falls back to the conversation's
	// own values when a caller leaves them empty).
	UserID  string `json:"userId,omitempty" firestore:"userId,omitempty"`
	BrandID string `json:"brandId,omitempty" firestore:"brandId,omitempty"`
	// ClientMsgID echoes the id the client generated for an optimistic user
	// bubble, so the Firestore snapshot can reconcile (dedupe) it on arrival.
	ClientMsgID string     `json:"clientMsgId,omitempty" firestore:"clientMsgId,omitempty"`
	Content     string     `json:"content" firestore:"content"`
	Model       string     `json:"model,omitempty" firestore:"model,omitempty"`
	FocusedText string     `json:"focusedText,omitempty" firestore:"focusedText,omitempty"`
	ImageURL    string     `json:"imageUrl,omitempty" firestore:"imageUrl,omitempty"`
	TokenCount  int        `json:"tokenCount,omitempty" firestore:"tokenCount,omitempty"`
	Timestamp   int64      `json:"timestamp" firestore:"timestamp"`
	Control     *AIControl `json:"control,omitempty" firestore:"control,omitempty"`
}

// AIControl is an optional structured answer control attached to an assistant
// message. It lets the AI ask a question with real UI controls instead of plain
// text — either a set of selectable options or a typed/validated input field.
// Available to every module (see ai package client tools ask_options/ask_input).
type AIControl struct {
	// Kind is "options" or "input".
	Kind string `json:"kind" firestore:"kind"`

	// Options-control fields.
	SelectionType string            `json:"selectionType,omitempty" firestore:"selectionType,omitempty"` // "single" | "multi"
	Options       []AIControlOption `json:"options,omitempty" firestore:"options,omitempty"`
	AllowCustom   bool              `json:"allowCustom,omitempty" firestore:"allowCustom,omitempty"`

	// Input-control fields.
	InputType   string `json:"inputType,omitempty" firestore:"inputType,omitempty"` // "text" | "phone" | "url" | "email"
	Placeholder string `json:"placeholder,omitempty" firestore:"placeholder,omitempty"`
	Optional    bool   `json:"optional,omitempty" firestore:"optional,omitempty"`
}

type AIControlOption struct {
	Label string `json:"label" firestore:"label"`
	Value string `json:"value" firestore:"value"`
}
