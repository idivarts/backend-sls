package streamsqs

type StreamWebhook struct {
	Type      string                         `json:"type"`
	CreatedAt string                         `json:"created_at"`
	User      StreamUser                     `json:"user"`
	Channels  map[string]ReminderChannelData `json:"channels"`
}

type StreamUser struct {
	ID         string   `json:"id"`
	Role       string   `json:"role"`
	CreatedAt  string   `json:"created_at"`
	UpdatedAt  string   `json:"updated_at"`
	LastActive string   `json:"last_active"`
	Banned     bool     `json:"banned"`
	Online     bool     `json:"online"`
	Teams      []string `json:"teams,omitempty"`
	Name       string   `json:"name"`
}

type ReminderChannelData struct {
	Channel  ReminderChannel `json:"channel"`
	Messages []StreamMessage `json:"messages"`
}

type ReminderChannel struct {
	ID            string          `json:"id"`
	Type          string          `json:"type"`
	CID           string          `json:"cid"`
	LastMessageAt string          `json:"last_message_at"`
	CreatedAt     string          `json:"created_at"`
	UpdatedAt     string          `json:"updated_at"`
	CreatedBy     StreamUser      `json:"created_by"`
	Frozen        bool            `json:"frozen"`
	Disabled      bool            `json:"disabled"`
	Members       []ChannelMember `json:"members"`
	MemberCount   int             `json:"member_count"`
	Config        ChannelConfig   `json:"config"`
	Name          string          `json:"name"`
}

type ChannelMember struct {
	UserID       string     `json:"user_id"`
	User         StreamUser `json:"user"`
	CreatedAt    string     `json:"created_at"`
	UpdatedAt    string     `json:"updated_at"`
	Banned       bool       `json:"banned"`
	ShadowBanned bool       `json:"shadow_banned"`
	Role         string     `json:"role"`
	ChannelRole  string     `json:"channel_role"`
}

type ChannelConfig struct {
	CreatedAt         string           `json:"created_at"`
	UpdatedAt         string           `json:"updated_at"`
	Name              string           `json:"name"`
	TypingEvents      bool             `json:"typing_events"`
	ReadEvents        bool             `json:"read_events"`
	ConnectEvents     bool             `json:"connect_events"`
	Search            bool             `json:"search"`
	Reactions         bool             `json:"reactions"`
	Replies           bool             `json:"replies"`
	Quotes            bool             `json:"quotes"`
	Mutes             bool             `json:"mutes"`
	Uploads           bool             `json:"uploads"`
	UrlEnrichment     bool             `json:"url_enrichment"`
	CustomEvents      bool             `json:"custom_events"`
	PushNotifications bool             `json:"push_notifications"`
	Reminders         bool             `json:"reminders"`
	MessageRetention  string           `json:"message_retention"`
	MaxMessageLength  int              `json:"max_message_length"`
	Automod           string           `json:"automod"`
	AutomodBehavior   string           `json:"automod_behavior"`
	Blocklist         string           `json:"blocklist"`
	BlocklistBehavior string           `json:"blocklist_behavior"`
	AutomodThresholds map[string]int   `json:"automod_thresholds"`
	Commands          []ChannelCommand `json:"commands"`
}

type ChannelCommand struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Args        string `json:"args"`
	Set         string `json:"set"`
}

type StreamMessage struct {
	ID              string      `json:"id"`
	Text            string      `json:"text"`
	HTML            string      `json:"html"`
	Type            string      `json:"type"`
	User            StreamUser  `json:"user"`
	Attachments     []any       `json:"attachments"`
	LatestReactions []any       `json:"latest_reactions"`
	OwnReactions    []any       `json:"own_reactions"`
	ReactionCounts  interface{} `json:"reaction_counts"`
	ReactionScores  interface{} `json:"reaction_scores"`
	ReplyCount      int         `json:"reply_count"`
	CID             string      `json:"cid"`
	CreatedAt       string      `json:"created_at"`
	UpdatedAt       string      `json:"updated_at"`
	Shadowed        bool        `json:"shadowed"`
	MentionedUsers  []any       `json:"mentioned_users"`
	Silent          bool        `json:"silent"`
	Pinned          bool        `json:"pinned"`
	PinnedAt        interface{} `json:"pinned_at"`
	PinnedBy        interface{} `json:"pinned_by"`
	PinExpires      interface{} `json:"pin_expires"`
}
