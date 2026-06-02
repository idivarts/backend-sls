package inbox

import (
	"log"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	instainterfaces "github.com/idivarts/backend-sls/pkg/interfaces/instaInterfaces"
)

// Webhook ingestion. Meta delivers events keyed by the platform account id
// (entry.id). We resolve that to a connected brand via socialAccountIndex and
// upsert/delete the corresponding inbox document. Events for accounts not in the
// index (e.g. the legacy chatbot pages) are ignored.

// resolveBrand maps a platform account id to its brand-connected account.
// Returns (brandID, account, ok). ok=false means "not an inbox account".
func resolveBrand(platformAccountID string) (string, *trendlymodels.SocialAccount, bool) {
	idx, err := trendlymodels.GetSocialAccountIndex(platformAccountID)
	if err != nil {
		return "", nil, false // unknown account — not ours
	}
	if idx.App != "brands" || idx.BrandID == "" {
		return "", nil, false // user-level inbox not supported in v1
	}
	acc, err := trendlymodels.GetBrandSocialAccount(idx.BrandID, idx.SocialID)
	if err != nil {
		log.Printf("inbox webhook: account %s/%s missing: %v", idx.BrandID, idx.SocialID, err)
		return "", nil, false
	}
	return idx.BrandID, acc, true
}

// channelForAccount returns the display channel for an event on a given account.
// A Facebook Page account also serves its linked IG Business Account, so events
// keyed by the IG id surface as the Instagram channel.
func channelForAccount(acc *trendlymodels.SocialAccount, platformAccountID string) trendlymodels.Platform {
	if acc.InstagramBusinessID != "" && platformAccountID == acc.InstagramBusinessID {
		return trendlymodels.PlatformInstagram
	}
	return acc.Platform
}

// IngestMessaging handles a DM event (entry.messaging[]). Handles inbound,
// outbound echoes, and unsend (deletion sync).
func IngestMessaging(platformAccountID string, m *instainterfaces.Messaging) {
	if m.Message == nil {
		return
	}
	brandID, acc, ok := resolveBrand(platformAccountID)
	if !ok {
		return
	}

	selfID := acc.PlatformAccountID
	isEcho := m.Message.IsEcho

	// Identify the contact (the non-self party in the 1:1 thread).
	contactID := m.Sender.ID
	if isEcho {
		contactID = m.Recipient.ID
	}
	if contactID == "" || contactID == selfID {
		return
	}

	// Deterministic conversation id per (account, contact). Webhook payloads do
	// not include Meta's conversation id, so we key by the participant pair.
	convID := "dmwh_" + acc.ID + "_" + contactID

	ts := m.Timestamp
	if ts == 0 {
		ts = time.Now().UnixMilli()
	}

	// ── Deletion sync: the user unsent a message. Remove it from our copy. ──
	if m.Message.IsDeleted {
		conv, err := trendlymodels.GetInboxConversation(brandID, convID)
		if err != nil || conv == nil {
			return
		}
		kept := conv.Messages[:0]
		for _, msg := range conv.Messages {
			if msg.ID != m.Message.Mid {
				kept = append(kept, msg)
			}
		}
		conv.Messages = kept
		if len(kept) > 0 {
			last := kept[len(kept)-1]
			conv.Preview = last.Text
			conv.LastActivityAt = last.SentAt
		}
		conv.UpdatedAt = time.Now().UnixMilli()
		if err := conv.Upsert(brandID); err != nil {
			log.Printf("inbox webhook: unsend upsert failed %s: %v", convID, err)
		}
		return
	}

	author := trendlymodels.InboxAuthorContact
	if isEcho {
		author = trendlymodels.InboxAuthorBusiness
	}
	newMsg := trendlymodels.InboxMessage{
		ID:     m.Message.Mid,
		Author: author,
		Text:   m.Message.Text,
		SentAt: ts,
	}

	conv, err := trendlymodels.GetInboxConversation(brandID, convID)
	if err != nil || conv == nil {
		conv = &trendlymodels.InboxConversation{
			ID:      convID,
			Kind:    trendlymodels.InboxKindDM,
			Channel: channelForAccount(acc, platformAccountID),
			Participant: trendlymodels.InboxParticipant{
				ID: contactID,
			},
			SocialID:            acc.ID,
			ExternalRecipientID: contactID,
			Messages:            []trendlymodels.InboxMessage{},
		}
	}
	// Avoid duplicating an echo we already optimistically stored.
	for _, existing := range conv.Messages {
		if existing.ID == newMsg.ID && newMsg.ID != "" {
			return
		}
	}
	conv.Messages = append(conv.Messages, newMsg)
	conv.Preview = newMsg.Text
	conv.LastActivityAt = ts
	conv.UpdatedAt = time.Now().UnixMilli()
	if !isEcho {
		conv.Unread = true
		conv.ReplyWindowExpiresAt = ts + replyWindowMs
	}
	if err := conv.Upsert(brandID); err != nil {
		log.Printf("inbox webhook: DM upsert failed %s: %v", convID, err)
	}
}

// IngestComment handles a comment/feed change event (entry.changes[]).
// Handles create, edit, hide/unhide, and delete (deletion sync).
func IngestComment(platformAccountID string, ch *instainterfaces.Change) {
	if ch.Field != "comments" && ch.Field != "feed" && ch.Field != "mentions" {
		return
	}
	// For FB `feed`, only comment items are relevant to the inbox.
	if ch.Field == "feed" && ch.Value.Item != "" && ch.Value.Item != "comment" {
		return
	}

	commentID := ch.Value.CommentExternalID()
	if commentID == "" {
		return
	}
	brandID, acc, ok := resolveBrand(platformAccountID)
	if !ok {
		return
	}
	convID := "cmt_" + commentID

	// ── Deletion sync: the user deleted their comment. Remove our copy. ──
	if ch.Value.IsRemoval() {
		if err := trendlymodels.DeleteInboxConversation(brandID, convID); err != nil {
			log.Printf("inbox webhook: comment delete failed %s: %v", convID, err)
		}
		return
	}

	// Hide / unhide → patch the flag if we already have the comment.
	if ch.Value.Verb == "hide" || ch.Value.Verb == "unhide" {
		if existing, err := trendlymodels.GetInboxConversation(brandID, convID); err == nil && existing.Comment != nil {
			_ = trendlymodels.UpdateInboxConversation(brandID, convID, []firestore.Update{
				{Path: "comment.hidden", Value: ch.Value.Verb == "hide"},
				{Path: "updatedAt", Value: time.Now().UnixMilli()},
			})
		}
		return
	}

	// Replies to our own comments are part of an existing thread — skip creating
	// a new top-level conversation for them.
	if ch.Value.IsReply() {
		return
	}

	now := time.Now().UnixMilli()
	text := ch.Value.CommentText()
	handle := firstNonEmpty(ch.Value.From.Username, ch.Value.From.Name)

	conv := &trendlymodels.InboxConversation{
		ID:      convID,
		Kind:    trendlymodels.InboxKindComment,
		Channel: channelForAccount(acc, platformAccountID),
		Participant: trendlymodels.InboxParticipant{
			ID:     ch.Value.From.ID,
			Name:   firstNonEmpty(ch.Value.From.Name, ch.Value.From.Username, "Unknown"),
			Handle: handle,
		},
		Preview:        text,
		LastActivityAt: now,
		Unread:         true,
		Post: &trendlymodels.InboxCommentPost{
			PostID: ch.Value.PostRef(),
		},
		Comment: &trendlymodels.InboxCommentPayload{
			Text:       text,
			AuthoredAt: now,
			Hidden:     false,
			Replies:    []trendlymodels.InboxMessage{},
		},
		SocialID:          acc.ID,
		ExternalCommentID: commentID,
		UpdatedAt:         now,
	}
	if err := conv.Upsert(brandID); err != nil {
		log.Printf("inbox webhook: comment upsert failed %s: %v", convID, err)
	}
}
