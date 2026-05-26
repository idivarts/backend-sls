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

func ListConversations(ctx context.Context, brandID, userID, module string, limit int) ([]trendlymodels.AIConversation, error) {
	if limit <= 0 {
		limit = 50
	}
	q := conversationsRef().
		Where("brandId", "==", brandID).
		Where("userId", "==", userID)
	if module != "" {
		q = q.Where("module", "==", module)
	}
	q = q.OrderBy("updatedAt", firestore.Desc).Limit(limit)

	iter := q.Documents(ctx)
	defer iter.Stop()

	var out []trendlymodels.AIConversation
	for {
		doc, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		var conv trendlymodels.AIConversation
		if err := doc.DataTo(&conv); err == nil {
			conv.ID = doc.Ref.ID
			out = append(out, conv)
		}
	}
	return out, nil
}

func AppendMessage(ctx context.Context, conversationID string, msg trendlymodels.AIMessage) (string, error) {
	if msg.Timestamp == 0 {
		msg.Timestamp = time.Now().UnixMilli()
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
		out = append(out, Message{Role: m.Role, Content: m.Content})
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
