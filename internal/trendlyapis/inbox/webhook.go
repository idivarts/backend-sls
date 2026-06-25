package inbox

import (
	"log"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	instainterfaces "github.com/idivarts/backend-sls/pkg/interfaces/instaInterfaces"
)

// Webhook ingestion. Meta delivers events keyed by the platform account id
// (entry.id). We resolve that to every brand that connected the account via
// socialAccountIndex and fan the event out, upserting/deleting the inbox document
// in each owner's store. Events for accounts not in the index (e.g. the legacy
// chatbot pages) are ignored.

// inboxTarget is one brand-connected account an event must be ingested into.
type inboxTarget struct {
	brandID string
	acc     *trendlymodels.SocialAccount
}

// resolveTargets maps a platform account id to every brand-connected account
// that should receive the event. The same account may be owned by multiple
// brands, so an event fans out to all of them. User-level inboxes are not
// supported in v1 and are skipped.
func resolveTargets(platformAccountID string) []inboxTarget {
	idx, err := trendlymodels.GetSocialAccountIndex(platformAccountID)
	if err != nil {
		return nil // unknown account — not ours
	}
	var targets []inboxTarget
	for _, o := range idx.AllOwners() {
		if o.App != "brands" || o.BrandID == "" {
			continue // user-level inbox not supported in v1
		}
		acc, err := trendlymodels.GetBrandSocialAccount(o.BrandID, o.SocialID)
		if err != nil {
			log.Printf("inbox webhook: account %s/%s missing: %v", o.BrandID, o.SocialID, err)
			continue
		}
		targets = append(targets, inboxTarget{brandID: o.BrandID, acc: acc})
	}
	return targets
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

// IngestMessaging handles a DM event (entry.messaging[]). It fans the event out
// to every brand that owns the account. Handles inbound, outbound echoes, unsend
// (deletion sync), and edits (message_edit).
func IngestMessaging(platformAccountID string, m *instainterfaces.Messaging) {
	// Edits arrive as a sibling message_edit object (no message field).
	if m.MessageEdit == nil && m.Message == nil {
		return
	}
	for _, t := range resolveTargets(platformAccountID) {
		if m.MessageEdit != nil {
			ingestMessageEditForBrand(t.brandID, t.acc, m)
		} else {
			ingestMessagingForBrand(t.brandID, t.acc, platformAccountID, m)
		}
	}
}

// ingestMessagingForBrand applies a DM event to a single brand's inbox.
func ingestMessagingForBrand(brandID string, acc *trendlymodels.SocialAccount, platformAccountID string, m *instainterfaces.Messaging) {
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

	log.Printf("inbox webhook: DM ingest brand=%s account=%s self=%s platform=%s contact=%s echo=%v mid=%s textLen=%d deleted=%v",
		brandID, acc.ID, selfID, acc.Platform, contactID, isEcho, m.Message.Mid, len(m.Message.Text), m.Message.IsDeleted)

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
	attURL, attType := firstAttachment(m.Message)
	newMsg := trendlymodels.InboxMessage{
		ID:             m.Message.Mid,
		Author:         author,
		Text:           m.Message.Text,
		SentAt:         ts,
		AttachmentURL:  attURL,
		AttachmentType: attType,
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
			if name, handle, avatar := fetchContactProfile(acc, tok.AccessToken, contactID, ""); name != "" || avatar != "" {
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

// ingestMessageEditForBrand applies a DM edit to a single brand's inbox: it
// locates the stored message by its mid and rewrites its text in place,
// preserving the rest of the thread. Edits for messages or conversations we
// don't track are ignored.
func ingestMessageEditForBrand(brandID string, acc *trendlymodels.SocialAccount, m *instainterfaces.Messaging) {
	if m.MessageEdit == nil || m.MessageEdit.Mid == "" {
		return
	}

	selfID := acc.PlatformAccountID
	// The contact is the non-self party (sender, unless the business edited its
	// own message, in which case the recipient is the contact).
	contactID := m.Sender.ID
	if contactID == selfID {
		contactID = m.Recipient.ID
	}
	if contactID == "" || contactID == selfID {
		return
	}

	convID := "dmwh_" + acc.ID + "_" + contactID
	conv, err := trendlymodels.GetInboxConversation(brandID, convID)
	if err != nil || conv == nil {
		return
	}

	updated := false
	for i := range conv.Messages {
		if conv.Messages[i].ID == m.MessageEdit.Mid {
			conv.Messages[i].Text = m.MessageEdit.Text
			// Refresh the list preview only if the edited message is the latest.
			if i == len(conv.Messages)-1 {
				conv.Preview = inboxMsgPreview(conv.Messages[i])
			}
			updated = true
			break
		}
	}
	if !updated {
		return
	}
	conv.UpdatedAt = time.Now().UnixMilli()
	if err := conv.Upsert(brandID); err != nil {
		log.Printf("inbox webhook: DM edit upsert failed %s: %v", convID, err)
	}
}

// IngestComment handles a comment/feed change event (entry.changes[]). It fans
// the event out to every brand that owns the account. Handles create, edit,
// hide/unhide, and delete (deletion sync).
func IngestComment(platformAccountID string, ch *instainterfaces.Change) {
	if ch.Field != "comments" && ch.Field != "feed" && ch.Field != "mentions" {
		return
	}
	// For FB `feed`, only comment items are relevant to the inbox.
	if ch.Field == "feed" && ch.Value.Item != "" && ch.Value.Item != "comment" {
		return
	}
	if ch.Value.CommentExternalID() == "" {
		return
	}
	for _, t := range resolveTargets(platformAccountID) {
		ingestCommentForBrand(t.brandID, t.acc, platformAccountID, ch)
	}
}

// ingestCommentForBrand applies a comment/feed event to a single brand's inbox.
func ingestCommentForBrand(brandID string, acc *trendlymodels.SocialAccount, platformAccountID string, ch *instainterfaces.Change) {
	commentID := ch.Value.CommentExternalID()
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

	// If we already track this top-level comment, this is an edit (FB `feed` verb
	// `edited`) or a re-delivery. Patch the text in place and preserve replies,
	// hidden state and read state — rebuilding the doc would wipe them.
	// (Instagram sends no comment edit/delete webhooks; this path is FB + retries.)
	if existing, err := trendlymodels.GetInboxConversation(brandID, convID); err == nil && existing != nil && existing.Comment != nil {
		_ = trendlymodels.UpdateInboxConversation(brandID, convID, []firestore.Update{
			{Path: "comment.text", Value: text},
			{Path: "preview", Value: text},
			{Path: "updatedAt", Value: now},
		})
		return
	}

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

// firstAttachment returns the URL and normalized type of the first attachment on
// a webhook DM, if any. Some attachments (e.g. unsupported media) carry a type
// but no URL — we still surface the type so the thread shows a placeholder.
func firstAttachment(msg *instainterfaces.Message) (url, typ string) {
	if msg == nil || msg.Attachments == nil || len(*msg.Attachments) == 0 {
		return "", ""
	}
	for _, a := range *msg.Attachments {
		if a.Payload.URL != "" {
			return a.Payload.URL, normalizeWebhookAttType(a.Type)
		}
	}
	return "", normalizeWebhookAttType((*msg.Attachments)[0].Type)
}

// normalizeWebhookAttType maps Meta's webhook attachment types (template, audio,
// file, image, share, story_mention, video, reel, ig_reel, post, ig_post) to the
// coarse set the frontend renders.
func normalizeWebhookAttType(t string) string {
	switch t {
	case "image":
		return "image"
	case "video", "reel", "ig_reel":
		return "video"
	case "audio":
		return "audio"
	case "share", "post", "ig_post":
		return "share"
	case "story_mention", "story":
		return "story"
	default:
		return "file"
	}
}

// inboxMsgPreview returns a one-line list preview for a DM, falling back to a
// type-aware placeholder when the message carries media but no text.
func inboxMsgPreview(m trendlymodels.InboxMessage) string {
	if m.Text != "" {
		return m.Text
	}
	if m.AttachmentURL != "" || m.AttachmentType != "" {
		return attachmentLabel(m.AttachmentType)
	}
	return ""
}

// attachmentLabel is the human label for a media-only message in list previews
// and (frontend mirrors this) attachment cards.
func attachmentLabel(typ string) string {
	switch typ {
	case "image":
		return "📷 Photo"
	case "video":
		return "🎬 Video"
	case "audio":
		return "🎤 Audio"
	case "share":
		return "🔗 Shared post"
	case "story":
		return "📖 Story"
	default:
		return "📎 Attachment"
	}
}
