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
	ClientMsgID string `json:"clientMsgId,omitempty" firestore:"clientMsgId,omitempty"`
	Content     string `json:"content" firestore:"content"`
	Model       string `json:"model,omitempty" firestore:"model,omitempty"`
	// FocusedText is the legacy plain-string focus (kept for back-compat). New
	// clients send the structured Focus list below; the string is still derived
	// and sent as a prompt fallback so an un-migrated backend/reader still works.
	FocusedText string `json:"focusedText,omitempty" firestore:"focusedText,omitempty"`
	// Focus is the structured "what the user pointed the AI at" for this message
	// (design element / strategy passage / calendar post / comment, with optional
	// inheritance). Persisted so the reference survives reload and can be rendered
	// back into the UI. Mirror of the frontend `Focus` type (types/focus.ts).
	Focus []AIFocus `json:"focus,omitempty" firestore:"focus,omitempty"`
	// ImageURL is the legacy single-image field (kept for back-compat). New code
	// uses Images (multi). On a user message Images are vision input the user
	// attached; on an assistant message they are generated/referenced image URLs.
	// Shared by the AIChatPanel-images and MediaStage threaded-generation flows —
	// always CloudFront/S3 URLs, never base64.
	ImageURL   string     `json:"imageUrl,omitempty" firestore:"imageUrl,omitempty"`
	Images     []string   `json:"images,omitempty" firestore:"images,omitempty"`
	TokenCount int        `json:"tokenCount,omitempty" firestore:"tokenCount,omitempty"`
	Timestamp  int64      `json:"timestamp" firestore:"timestamp"`
	Control    *AIControl `json:"control,omitempty" firestore:"control,omitempty"`
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

// AIFocus mirrors the frontend `Focus` (types/focus.ts): a structured target the
// user pointed the AI at, plus its human display label.
type AIFocus struct {
	ID        string      `json:"id,omitempty" firestore:"id,omitempty"`
	FocusText string      `json:"focusText,omitempty" firestore:"focusText,omitempty"`
	FocusArea AIFocusArea `json:"focusArea" firestore:"focusArea"`
}

// AIFocusArea mirrors the frontend `FocusArea` discriminated union. Fields are
// flattened with a `Type` discriminator; only the fields relevant to a given
// Type are populated. `Inherits` supports a comment focus that itself points at
// another area (a design element, a strategy passage, …).
type AIFocusArea struct {
	Type string `json:"type" firestore:"type"`

	// content / design / calendar
	ContentID   string `json:"contentId,omitempty" firestore:"contentId,omitempty"`
	Title       string `json:"title,omitempty" firestore:"title,omitempty"`
	ContentType string `json:"contentType,omitempty" firestore:"contentType,omitempty"`
	Date        string `json:"date,omitempty" firestore:"date,omitempty"`

	// design-element
	RevisionID string `json:"revisionId,omitempty" firestore:"revisionId,omitempty"`
	ElementID  string `json:"elementId,omitempty" firestore:"elementId,omitempty"`
	SlideIndex *int   `json:"slideIndex,omitempty" firestore:"slideIndex,omitempty"`
	SlideCount *int   `json:"slideCount,omitempty" firestore:"slideCount,omitempty"`
	DocType    string `json:"docType,omitempty" firestore:"docType,omitempty"`
	Text       string `json:"text,omitempty" firestore:"text,omitempty"`

	// strategy-snippet
	StrategyID  string `json:"strategyId,omitempty" firestore:"strategyId,omitempty"`
	Snippet     string `json:"snippet,omitempty" firestore:"snippet,omitempty"`
	AnchorStart *int   `json:"anchorStart,omitempty" firestore:"anchorStart,omitempty"`
	AnchorEnd   *int   `json:"anchorEnd,omitempty" firestore:"anchorEnd,omitempty"`

	// comment
	CommentID string       `json:"commentId,omitempty" firestore:"commentId,omitempty"`
	Inherits  *AIFocusArea `json:"inherits,omitempty" firestore:"inherits,omitempty"`
}
