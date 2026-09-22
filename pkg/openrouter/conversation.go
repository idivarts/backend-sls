package openrouter

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/google/uuid"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"google.golang.org/api/iterator"
)

const (
	conversationsCollection = "ai_conversations"
	messagesSubcollection   = "messages"
)

func conversationsRef() *firestore.CollectionRef {
	return firestoredb.Client.Collection(conversationsCollection)
}

func messagesRef(conversationID string) *firestore.CollectionRef {
	return conversationsRef().Doc(conversationID).Collection(messagesSubcollection)
}

func CreateConversation(ctx context.Context, brandID, userID, module, contextID, model, title string) (*trendlymodels.AIConversation, error) {
	now := time.Now().UnixMilli()
	id := uuid.NewString()
	conv := trendlymodels.AIConversation{
		ID:           id,
		BrandID:      brandID,
		UserID:       userID,
		Module:       module,
		ContextID:    contextID,
		Title:        title,
		CurrentModel: model,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if _, err := conversationsRef().Doc(id).Set(ctx, conv); err != nil {
		return nil, err
	}
	return &conv, nil
}

// CountConversationsByBrand returns how many AI conversations a brand has
// started. Conversations live in one flat top-level collection keyed by
// brandId (not under the brand), so this filters rather than scoping a
// subcollection. Used by the admin Brand CRM.
func CountConversationsByBrand(ctx context.Context, brandID string) (int, error) {
	if brandID == "" {
		return 0, fmt.Errorf("CountConversationsByBrand: empty brandID")
	}
	return trendlymodels.CountQuery(ctx, conversationsRef().Where("brandId", "==", brandID))
}

func GetConversation(ctx context.Context, conversationID string) (*trendlymodels.AIConversation, error) {
	snap, err := conversationsRef().Doc(conversationID).Get(ctx)
	if err != nil {
		return nil, err
	}
	var conv trendlymodels.AIConversation
	if err := snap.DataTo(&conv); err != nil {
		return nil, err
	}
	conv.ID = snap.Ref.ID
	return &conv, nil
}

func DeleteConversation(ctx context.Context, conversationID string) error {
	iter := messagesRef(conversationID).Documents(ctx)
	defer iter.Stop()
	batch := firestoredb.Client.Batch()
	count := 0
	for {
		doc, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return err
		}
		batch.Delete(doc.Ref)
		count++
		if count >= 400 {
			if _, err := batch.Commit(ctx); err != nil {
				return err
			}
			batch = firestoredb.Client.Batch()
			count = 0
		}
	}
	if count > 0 {
		if _, err := batch.Commit(ctx); err != nil {
			return err
		}
	}
	_, err := conversationsRef().Doc(conversationID).Delete(ctx)
	return err
}

func AppendMessage(ctx context.Context, conversationID string, msg trendlymodels.AIMessage) (string, error) {
	if msg.Timestamp == 0 {
		msg.Timestamp = time.Now().UnixMilli()
	}
	// Every message carries the owning userId/brandId so the Firestore client
	// read rules can authorize it without a parent get(). Hot callers stamp
	// these directly (no extra read); fall back to the conversation doc only
	// when a caller left them empty.
	if msg.UserID == "" || msg.BrandID == "" {
		if conv, err := GetConversation(ctx, conversationID); err == nil {
			if msg.UserID == "" {
				msg.UserID = conv.UserID
			}
			if msg.BrandID == "" {
				msg.BrandID = conv.BrandID
			}
		}
	}
	doc, _, err := messagesRef(conversationID).Add(ctx, msg)
	if err != nil {
		return "", err
	}
	_, _ = conversationsRef().Doc(conversationID).Update(ctx, []firestore.Update{
		{Path: "updatedAt", Value: time.Now().UnixMilli()},
	})
	return doc.ID, nil
}

func LoadHistory(ctx context.Context, conversationID string) ([]trendlymodels.AIMessage, error) {
	iter := messagesRef(conversationID).OrderBy("timestamp", firestore.Asc).Documents(ctx)
	defer iter.Stop()

	var out []trendlymodels.AIMessage
	for {
		doc, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		var msg trendlymodels.AIMessage
		if err := doc.DataTo(&msg); err == nil {
			out = append(out, msg)
		}
	}
	return out, nil
}

func UpdateConversationTitle(ctx context.Context, conversationID, title string) error {
	_, err := conversationsRef().Doc(conversationID).Update(ctx, []firestore.Update{
		{Path: "title", Value: title},
		{Path: "updatedAt", Value: time.Now().UnixMilli()},
	})
	return err
}

// RequestCancel marks the conversation so its in-flight AI turn cooperatively
// aborts. The stop signal arrives on a separate WS (Lambda) invocation from the
// one running the turn, so the marker is the cross-invocation channel: the
// streaming loop polls it (throttled) and bails when it sees a request newer than
// its own start time.
func RequestCancel(ctx context.Context, conversationID string) error {
	_, err := conversationsRef().Doc(conversationID).Update(ctx, []firestore.Update{
		{Path: "cancelRequestedAt", Value: time.Now().UnixMilli()},
	})
	return err
}

// GetCancelRequestedAt returns the conversation's current cancel-request
// timestamp (0 when none). Read by the streaming loop to decide whether to abort.
func GetCancelRequestedAt(ctx context.Context, conversationID string) (int64, error) {
	snap, err := conversationsRef().Doc(conversationID).Get(ctx)
	if err != nil {
		return 0, err
	}
	var conv trendlymodels.AIConversation
	if err := snap.DataTo(&conv); err != nil {
		return 0, err
	}
	return conv.CancelRequestedAt, nil
}

func UpdateConversationModel(ctx context.Context, conversationID, model string) error {
	_, err := conversationsRef().Doc(conversationID).Update(ctx, []firestore.Update{
		{Path: "currentModel", Value: model},
		{Path: "updatedAt", Value: time.Now().UnixMilli()},
	})
	return err
}

func ToOpenRouterMessages(history []trendlymodels.AIMessage) []Message {
	out := make([]Message, 0, len(history))
	for _, m := range history {
		content := m.Content
		// Carry a user turn's focus into its content so the referenced target
		// (element/slide/content/comment) persists across turns — otherwise a
		// later "move this" loses the reference and the model asks "which one?".
		if m.Role == "user" {
			if note := m.FocusNote(); note != "" {
				content = note + "\n" + content
			}
		}
		// Replay a user turn's attached images as multimodal vision input so the
		// model keeps visual context across the thread. Assistant/tool turns stay
		// text-only (their generated images are surfaced as URLs in the prose).
		if m.Role == "user" && len(m.Images) > 0 {
			out = append(out, UserMessageWithImages(content, m.Images))
			continue
		}
		out = append(out, Message{Role: m.Role, Content: content})
	}
	return out
}

func MustExist(conv *trendlymodels.AIConversation, err error) error {
	if err != nil {
		return err
	}
	if conv == nil {
		return fmt.Errorf("conversation not found")
	}
	return nil
}
