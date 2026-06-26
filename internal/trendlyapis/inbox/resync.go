package inbox

import (
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/instagram"
	"github.com/idivarts/backend-sls/pkg/facebook"
)

// Unit-level resyncs: refresh exactly one stale item (an expired avatar/attachment
// URL, a drifted media count, an outdated username) without rebuilding the whole
// cache. Each writes to Firestore; the frontend's listeners surface the change live.

// ResyncProfile re-fetches a conversation contact's display profile (name, handle,
// avatar) and updates just that conversation's participant.
func ResyncProfile(brandID, convID string) error {
	conv, err := trendlymodels.GetInboxConversation(brandID, convID)
	if err != nil {
		return err
	}
	sa, err := loadServingAccount(brandID, conv.SocialID)
	if err != nil {
		return err
	}
	name, handle, avatar := fetchContactProfile(sa.account, conv.Channel, sa.token, conv.Participant.ID, conv.Participant.Handle)

	updates := []firestore.Update{{Path: "updatedAt", Value: time.Now().UnixMilli()}}
	if name != "" {
		updates = append(updates, firestore.Update{Path: "participant.name", Value: name})
	}
	if handle != "" {
		updates = append(updates, firestore.Update{Path: "participant.handle", Value: handle})
	}
	if avatar != "" {
		updates = append(updates, firestore.Update{Path: "participant.avatarUrl", Value: avatar})
	}
	return trendlymodels.UpdateInboxConversation(brandID, convID, updates)
}

// ResyncThread re-pulls one DM thread's messages from Meta. Facebook (incl.
// IG-via-Page) fetches just this thread when we know its Meta conversation id;
// Instagram-Login has no single-conversation fetch, so it falls back to re-syncing
// the whole account (this thread is included).
func ResyncThread(brandID, convID string) error {
	conv, err := trendlymodels.GetInboxConversation(brandID, convID)
	if err != nil {
		return err
	}
	if conv.Kind != trendlymodels.InboxKindDM {
		return fmt.Errorf("ResyncThread: %s is not a DM", convID)
	}
	sa, err := loadServingAccount(brandID, conv.SocialID)
	if err != nil {
		return err
	}
	acc := sa.account

	if acc.Platform == trendlymodels.PlatformFacebook && conv.ExternalConversationID != "" {
		msgsData, err := facebook.GetMessagesWithPagination(conv.ExternalConversationID, "", 25, sa.token)
		if err != nil {
			return err
		}
		rebuildThreadMessages(acc, conv, msgsData.Data)
		conv.UpdatedAt = time.Now().UnixMilli()
		return conv.Upsert(brandID)
	}

	return syncAccountDMs(brandID, acc, sa.token)
}

// rebuildThreadMessages replaces a conversation's messages from a fresh Meta
// pull (newest-first) and recomputes preview / last-activity / reply-window.
func rebuildThreadMessages(acc *trendlymodels.SocialAccount, conv *trendlymodels.InboxConversation, data []facebook.Message) {
	selfID := acc.PlatformAccountID
	msgs := make([]trendlymodels.InboxMessage, 0, len(data))
	var lastAt, lastInboundAt int64
	var preview string
	for i := len(data) - 1; i >= 0; i-- {
		msg := mapMessengerMessage(acc, selfID, data[i])
		msgs = append(msgs, msg)
		if msg.SentAt > lastAt {
			lastAt = msg.SentAt
			preview = inboxMsgPreview(msg)
		}
		if msg.Author == trendlymodels.InboxAuthorContact && msg.SentAt > lastInboundAt {
			lastInboundAt = msg.SentAt
		}
	}
	conv.Messages = msgs
	if lastAt > 0 {
		conv.LastActivityAt = lastAt
		conv.Preview = preview
	}
	if lastInboundAt > 0 {
		conv.ReplyWindowExpiresAt = lastInboundAt + replyWindowMs
	}
}

// ResyncMessage re-fetches a single message (e.g. an expired attachment URL) and
// updates it in place. Instagram-Login has no single-message fetch, so it falls
// back to a full thread resync.
func ResyncMessage(brandID, convID, msgID string) error {
	conv, err := trendlymodels.GetInboxConversation(brandID, convID)
	if err != nil {
		return err
	}
	sa, err := loadServingAccount(brandID, conv.SocialID)
	if err != nil {
		return err
	}
	if sa.account.Platform == trendlymodels.PlatformInstagram {
		return ResyncThread(brandID, convID)
	}

	full, err := facebook.GetMessageInfo(msgID, sa.token)
	if err != nil {
		return err
	}
	updated := mapMessengerMessage(sa.account, sa.account.PlatformAccountID, *full)

	found := false
	for i := range conv.Messages {
		if conv.Messages[i].ID == msgID {
			conv.Messages[i].Text = updated.Text
			conv.Messages[i].AttachmentURL = updated.AttachmentURL
			conv.Messages[i].AttachmentType = updated.AttachmentType
			conv.Messages[i].AttachmentThumbURL = updated.AttachmentThumbURL
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("ResyncMessage: message %s not found in %s", msgID, convID)
	}
	conv.UpdatedAt = time.Now().UnixMilli()
	return conv.Upsert(brandID)
}

// ResyncMediaItem re-fetches one published media (image, comment + like counts)
// and upserts its Firestore doc.
func ResyncMediaItem(brandID, mediaID, socialID, channel string) error {
	sa, err := loadServingAccount(brandID, socialID)
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()

	var doc *trendlymodels.InboxMediaDoc
	if channel == trendlymodels.PlatformFacebook {
		p, err := facebook.GetPostByID(mediaID, sa.token)
		if err != nil {
			return err
		}
		doc = &trendlymodels.InboxMediaDoc{
			ID:            p.ID,
			Channel:       trendlymodels.PlatformFacebook,
			SocialID:      socialID,
			ThumbnailURL:  p.FullPicture,
			Caption:       p.Message,
			Permalink:     p.PermalinkURL,
			Timestamp:     p.CreatedTime.UnixMilli(),
			CommentsCount: p.CommentCount(),
			LikeCount:     p.LikeCount(),
			UpdatedAt:     now,
		}
	} else {
		m, err := instagram.GetMediaByID(mediaID, sa.token, graphTypeForIG(sa.account))
		if err != nil {
			return err
		}
		thumb := m.ThumbnailURL
		if thumb == "" {
			thumb = m.MediaURL
		}
		doc = &trendlymodels.InboxMediaDoc{
			ID:            m.ID,
			Channel:       trendlymodels.PlatformInstagram,
			SocialID:      socialID,
			ThumbnailURL:  thumb,
			Caption:       m.Caption,
			Permalink:     m.Permalink,
			Timestamp:     m.Timestamp.UnixMilli(),
			CommentsCount: m.CommentsCount,
			LikeCount:     m.LikeCount,
			UpdatedAt:     now,
		}
	}
	return doc.Upsert(brandID)
}
