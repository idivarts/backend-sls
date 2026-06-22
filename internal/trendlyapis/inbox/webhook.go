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
			conv.Preview = inboxMsgPreview(last)
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
		ID:            m.Message.Mid,
		Author:        author,
		Text:          m.Message.Text,
		SentAt:        ts,
		AttachmentURL: firstAttachmentURL(m.Message),
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
	// Webhook payloads carry only the contact's id — no name or avatar. Hydrate
	// the participant's display profile from Meta so the inbox doesn't fall back
	// to showing the page name with an empty avatar.
	if conv.Participant.Name == "" || conv.Participant.AvatarURL == "" {
		if tok, terr := trendlymodels.GetBrandSocialToken(brandID, acc.ID); terr == nil {
			if name, handle, avatar := fetchContactProfile(acc, tok.AccessToken, contactID); name != "" || avatar != "" {
				if conv.Participant.Name == "" && name != "" {
					conv.Participant.Name = name
				}
				if conv.Participant.Handle == "" && handle != "" {
					conv.Participant.Handle = handle
				}
				if conv.Participant.AvatarURL == "" && avatar != "" {
					conv.Participant.AvatarURL = avatar
				}
			}
		}
		if conv.Participant.Name == "" {
			conv.Participant.Name = "Unknown"
		}
	}
	// Avoid duplicating an echo we already optimistically stored.
	for _, existing := range conv.Messages {
		if existing.ID == newMsg.ID && newMsg.ID != "" {
			return
		}
	}
	conv.Messages = append(conv.Messages, newMsg)
	conv.Preview = inboxMsgPreview(newMsg)
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

	// Replies are part of an existing thread — append to the parent conversation
	// rather than creating a new top-level conversation. (IG comment threads are
	// one level deep: a reply's parent_id is always the top-level comment.)
	if ch.Value.IsReply() {
		ingestCommentReply(brandID, acc, ch)
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

// ingestCommentReply appends an inbound reply to the parent comment thread. The
// parent conversation must already be cached (it was created when the top-level
// comment arrived); replies whose parent we don't track are ignored.
func ingestCommentReply(brandID string, acc *trendlymodels.SocialAccount, ch *instainterfaces.Change) {
	parentConvID := "cmt_" + ch.Value.ParentID
	conv, err := trendlymodels.GetInboxConversation(brandID, parentConvID)
	if err != nil || conv == nil || conv.Comment == nil {
		return
	}

	replyID := ch.Value.CommentExternalID()
	// Dedupe a reply we already stored (e.g. our own optimistic reply).
	for _, r := range conv.Comment.Replies {
		if r.ID == replyID && replyID != "" {
			return
		}
	}

	now := time.Now().UnixMilli()
	author := trendlymodels.InboxAuthorContact
	if isSelfAuthor(acc, ch.Value.From.ID) {
		author = trendlymodels.InboxAuthorBusiness
	}
	reply := trendlymodels.InboxMessage{
		ID:     replyID,
		Author: author,
		Text:   ch.Value.CommentText(),
		SentAt: now,
	}
	conv.Comment.Replies = append(conv.Comment.Replies, reply)
	conv.Preview = reply.Text
	conv.LastActivityAt = now
	if author == trendlymodels.InboxAuthorContact {
		conv.Unread = true
	}
	conv.UpdatedAt = now
	if err := conv.Upsert(brandID); err != nil {
		log.Printf("inbox webhook: comment reply upsert failed %s: %v", parentConvID, err)
	}
}

// isSelfAuthor reports whether a comment/reply author is the connected account
// itself (so it surfaces as a "business" reply rather than an inbound contact).
func isSelfAuthor(acc *trendlymodels.SocialAccount, fromID string) bool {
	if fromID == "" {
		return false
	}
	return fromID == acc.PlatformAccountID ||
		(acc.InstagramBusinessID != "" && fromID == acc.InstagramBusinessID)
}

// firstAttachmentURL returns the URL of the first usable attachment on a DM, if any.
func firstAttachmentURL(msg *instainterfaces.Message) string {
	if msg == nil || msg.Attachments == nil {
		return ""
	}
	for _, a := range *msg.Attachments {
		if a.Payload.URL != "" {
			return a.Payload.URL
		}
	}
	return ""
}

// inboxMsgPreview returns a one-line list preview for a DM, falling back to an
// attachment placeholder when the message carries media but no text.
func inboxMsgPreview(m trendlymodels.InboxMessage) string {
	if m.Text != "" {
		return m.Text
	}
	if m.AttachmentURL != "" {
		return "📎 Attachment"
	}
	return ""
}
