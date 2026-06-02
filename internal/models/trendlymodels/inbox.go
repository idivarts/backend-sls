package trendlymodels

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"google.golang.org/api/iterator"
)

// ─── Inbox store (brands/{brandId}/inbox/{conversationId}) ────────────────────
//
// Source of truth for inbox reads. Populated lazily (read-through from Meta on
// cache-miss) and kept in sync by webhooks (new + deleted events). JSON tags
// match the frontend `UseInboxResult` contract verbatim so handlers can return
// these structs directly.

type InboxKind = string

const (
	InboxKindDM      InboxKind = "dm"
	InboxKindComment InboxKind = "comment"
)

type InboxAuthor = string

const (
	InboxAuthorContact  InboxAuthor = "contact"
	InboxAuthorBusiness InboxAuthor = "business"
)

type InboxParticipant struct {
	ID        string `json:"id" firestore:"id"`
	Name      string `json:"name" firestore:"name"`
	Handle    string `json:"handle,omitempty" firestore:"handle,omitempty"`
	AvatarURL string `json:"avatarUrl,omitempty" firestore:"avatarUrl,omitempty"`
}

type InboxMessage struct {
	ID            string      `json:"id" firestore:"id"`
	Author        InboxAuthor `json:"author" firestore:"author"`
	Text          string      `json:"text" firestore:"text"`
	SentAt        int64       `json:"sentAt" firestore:"sentAt"` // epoch ms
	AttachmentURL string      `json:"attachmentUrl,omitempty" firestore:"attachmentUrl,omitempty"`
}

type InboxCommentPost struct {
	PostID       string `json:"postId" firestore:"postId"`
	ThumbnailURL string `json:"thumbnailUrl,omitempty" firestore:"thumbnailUrl,omitempty"`
	Caption      string `json:"caption,omitempty" firestore:"caption,omitempty"`
}

type InboxCommentPayload struct {
	Text       string         `json:"text" firestore:"text"`
	AuthoredAt int64          `json:"authoredAt" firestore:"authoredAt"`
	Hidden     bool           `json:"hidden" firestore:"hidden"`
	Replies    []InboxMessage `json:"replies" firestore:"replies"`
}

type InboxContact struct {
	FollowerCount       int64  `json:"followerCount,omitempty" firestore:"followerCount,omitempty"`
	Bio                 string `json:"bio,omitempty" firestore:"bio,omitempty"`
	Location            string `json:"location,omitempty" firestore:"location,omitempty"`
	IsTrendlyInfluencer bool   `json:"isTrendlyInfluencer,omitempty" firestore:"isTrendlyInfluencer,omitempty"`
	LinkedInfluencerID  string `json:"linkedInfluencerId,omitempty" firestore:"linkedInfluencerId,omitempty"`
}

type InboxConversation struct {
	ID        string    `json:"id" firestore:"id"`
	Kind      InboxKind `json:"kind" firestore:"kind"`
	Channel   Platform  `json:"channel" firestore:"channel"` // "instagram" | "facebook"

	Participant InboxParticipant `json:"participant" firestore:"participant"`

	Preview        string `json:"preview" firestore:"preview"`
	LastActivityAt int64  `json:"lastActivityAt" firestore:"lastActivityAt"` // epoch ms — list sort key
	Unread         bool   `json:"unread" firestore:"unread"`

	// DM-only: epoch ms when the 24h reply window closes (0/omitted for comments).
	ReplyWindowExpiresAt int64 `json:"replyWindowExpiresAt,omitempty" firestore:"replyWindowExpiresAt,omitempty"`

	// DM payload.
	Messages []InboxMessage `json:"messages,omitempty" firestore:"messages,omitempty"`

	// Comment payload.
	Post    *InboxCommentPost    `json:"post,omitempty" firestore:"post,omitempty"`
	Comment *InboxCommentPayload `json:"comment,omitempty" firestore:"comment,omitempty"`

	Contact *InboxContact `json:"contact,omitempty" firestore:"contact,omitempty"`

	// ── Internal routing fields (used server-side; harmless to the frontend) ──
	// SocialID is the connected SocialAccount whose token serves this conversation.
	SocialID string `json:"socialId,omitempty" firestore:"socialId,omitempty"`
	// ExternalConversationID is the Meta conversation id (DMs).
	ExternalConversationID string `json:"-" firestore:"externalConversationId,omitempty"`
	// ExternalCommentID is the Meta comment id (comments).
	ExternalCommentID string `json:"-" firestore:"externalCommentId,omitempty"`
	// ExternalRecipientID is the platform-scoped id to send a DM reply to.
	ExternalRecipientID string `json:"-" firestore:"externalRecipientId,omitempty"`

	UpdatedAt int64 `json:"-" firestore:"updatedAt"`
}

func brandInboxCollection(brandID string) string {
	return fmt.Sprintf("brands/%s/inbox", brandID)
}

// Upsert writes (creates or overwrites) the conversation document.
func (conv *InboxConversation) Upsert(brandID string) error {
	if conv.ID == "" {
		return fmt.Errorf("InboxConversation.Upsert: empty ID")
	}
	_, err := firestoredb.Client.
		Collection(brandInboxCollection(brandID)).
		Doc(conv.ID).
		Set(context.Background(), conv)
	return err
}

// GetInboxConversation reads a single conversation.
func GetInboxConversation(brandID, id string) (*InboxConversation, error) {
	doc, err := firestoredb.Client.
		Collection(brandInboxCollection(brandID)).
		Doc(id).
		Get(context.Background())
	if err != nil {
		return nil, err
	}
	var conv InboxConversation
	if err := doc.DataTo(&conv); err != nil {
		return nil, err
	}
	return &conv, nil
}

// ListInboxConversations returns all conversations for a brand, newest first.
func ListInboxConversations(brandID string) ([]InboxConversation, error) {
	iter := firestoredb.Client.
		Collection(brandInboxCollection(brandID)).
		OrderBy("lastActivityAt", firestore.Desc).
		Documents(context.Background())
	defer iter.Stop()

	out := make([]InboxConversation, 0)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var conv InboxConversation
		if err := doc.DataTo(&conv); err != nil {
			return nil, err
		}
		out = append(out, conv)
	}
	return out, nil
}

// InboxQuery describes an optional single-dimension server-side filter. Only one
// dimension should be set per call so the query stays within the two-field
// composite indexes (<field> ASC, lastActivityAt DESC) defined in
// firestore.indexes.json.
type InboxQuery struct {
	UnreadOnly bool
	Kind       InboxKind // "dm" | "comment" | ""
	Channel    Platform  // "instagram" | "facebook" | ""
}

// ListInboxConversationsFiltered returns conversations matching a single filter
// dimension, newest first. With an empty query it is equivalent to
// ListInboxConversations.
func ListInboxConversationsFiltered(brandID string, q InboxQuery) ([]InboxConversation, error) {
	query := firestoredb.Client.Collection(brandInboxCollection(brandID)).Query
	if q.UnreadOnly {
		query = query.Where("unread", "==", true)
	}
	if q.Kind != "" {
		query = query.Where("kind", "==", q.Kind)
	}
	if q.Channel != "" {
		query = query.Where("channel", "==", q.Channel)
	}
	query = query.OrderBy("lastActivityAt", firestore.Desc)

	iter := query.Documents(context.Background())
	defer iter.Stop()

	out := make([]InboxConversation, 0)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var conv InboxConversation
		if err := doc.DataTo(&conv); err != nil {
			return nil, err
		}
		out = append(out, conv)
	}
	return out, nil
}

// CountUnreadInboxConversations returns the number of unread conversations.
func CountUnreadInboxConversations(brandID string) (int, error) {
	docs, err := firestoredb.Client.
		Collection(brandInboxCollection(brandID)).
		Where("unread", "==", true).
		Select().
		Documents(context.Background()).
		GetAll()
	if err != nil {
		return 0, err
	}
	return len(docs), nil
}

// CountInboxConversations returns how many conversations a brand has cached.
func CountInboxConversations(brandID string) (int, error) {
	docs, err := firestoredb.Client.
		Collection(brandInboxCollection(brandID)).
		Select().
		Documents(context.Background()).
		GetAll()
	if err != nil {
		return 0, err
	}
	return len(docs), nil
}

// UpdateInboxConversation applies a partial update.
func UpdateInboxConversation(brandID, id string, fields []firestore.Update) error {
	_, err := firestoredb.Client.
		Collection(brandInboxCollection(brandID)).
		Doc(id).
		Update(context.Background(), fields)
	return err
}

// DeleteInboxConversation removes a conversation document (used by deletion sync
// and the comment-delete action).
func DeleteInboxConversation(brandID, id string) error {
	_, err := firestoredb.Client.
		Collection(brandInboxCollection(brandID)).
		Doc(id).
		Delete(context.Background())
	return err
}
