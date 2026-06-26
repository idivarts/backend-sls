package inbox

import (
	"fmt"
	"log"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/internal/socialsync"
	"github.com/idivarts/backend-sls/pkg/instagram"
	"github.com/idivarts/backend-sls/pkg/facebook"
)

const replyWindowMs = int64(24 * 60 * 60 * 1000)

// ConnectedAccount is the inbox view of a connected social account
// (matches the frontend `ConnectedInboxAccount`).
type ConnectedAccount struct {
	ID        string `json:"id"`
	Channel   string `json:"channel"` // "instagram" | "facebook"
	Name      string `json:"name"`
	Handle    string `json:"handle,omitempty"`
	AvatarURL string `json:"avatarUrl,omitempty"`
}

// servingAccount bundles a connected account with its (decrypted) token.
type servingAccount struct {
	account *trendlymodels.SocialAccount
	token   string
}

// isInboxChannel reports whether a platform participates in the inbox (Meta only).
func isInboxChannel(p trendlymodels.Platform) bool {
	return p == trendlymodels.PlatformInstagram || p == trendlymodels.PlatformFacebook
}

// ListAccounts returns the brand's Meta-connected accounts for the inbox.
func ListAccounts(brandID string) ([]ConnectedAccount, error) {
	socials, err := trendlymodels.ListBrandSocialAccounts(brandID)
	if err != nil {
		return nil, err
	}
	out := make([]ConnectedAccount, 0, len(socials))
	for i := range socials {
		s := socials[i]
		if !isInboxChannel(s.Platform) {
			continue
		}
		out = append(out, ConnectedAccount{
			ID:        s.ID,
			Channel:   s.Platform,
			Name:      s.DisplayName,
			Handle:    s.Username,
			AvatarURL: s.ProfileImageURL,
		})
		// A Facebook Page with a linked IG Business Account also serves the
		// Instagram channel — surface it so the UI shows IG as connected.
		if s.Platform == trendlymodels.PlatformFacebook && s.InstagramBusinessID != "" {
			out = append(out, ConnectedAccount{
				ID:        s.ID,
				Channel:   trendlymodels.PlatformInstagram,
				Name:      s.DisplayName,
				AvatarURL: s.ProfileImageURL,
			})
		}
	}
	return out, nil
}

// loadServingAccount loads a connected account + token by social id.
func loadServingAccount(brandID, socialID string) (*servingAccount, error) {
	acc, err := trendlymodels.GetBrandSocialAccount(brandID, socialID)
	if err != nil {
		return nil, fmt.Errorf("loadServingAccount: account %s: %w", socialID, err)
	}
	tok, err := trendlymodels.GetBrandSocialToken(brandID, socialID)
	if err != nil {
		return nil, fmt.Errorf("loadServingAccount: token %s: %w", socialID, err)
	}
	return &servingAccount{account: acc, token: tok.AccessToken}, nil
}

// GetConversations serves the brand's conversations from Firestore, performing a
// one-time read-through sync from Meta on a cold cache. Sync is best-effort: if
// it fails (e.g. App Review not yet granted) we still return whatever is cached.
func GetConversations(brandID string) ([]trendlymodels.InboxConversation, error) {
	count, err := trendlymodels.CountInboxConversations(brandID)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		if syncErr := enqueueOrRun(brandID, socialsync.OpInboxSync); syncErr != nil {
			log.Printf("inbox: read-through enqueue failed for brand %s: %v", brandID, syncErr)
		}
	}
	return trendlymodels.ListInboxConversations(brandID)
}

// filterToQuery maps a single UI filter token to an indexed server-side query.
// Unknown / "all" / "" → no filter. Only one dimension is ever applied so the
// query stays within the two-field composite indexes.
func filterToQuery(filter string) trendlymodels.InboxQuery {
	switch filter {
	case "unread":
		return trendlymodels.InboxQuery{UnreadOnly: true}
	case "dm":
		return trendlymodels.InboxQuery{Kind: trendlymodels.InboxKindDM}
	case "comment":
		return trendlymodels.InboxQuery{Kind: trendlymodels.InboxKindComment}
	case "instagram":
		return trendlymodels.InboxQuery{Channel: trendlymodels.PlatformInstagram}
	case "facebook":
		return trendlymodels.InboxQuery{Channel: trendlymodels.PlatformFacebook}
	default:
		return trendlymodels.InboxQuery{}
	}
}

// GetConversationsFiltered serves conversations for a single filter dimension,
// performing the same cold-cache read-through as GetConversations. An empty
// filter returns everything (identical to GetConversations).
func GetConversationsFiltered(brandID, filter string) ([]trendlymodels.InboxConversation, error) {
	count, err := trendlymodels.CountInboxConversations(brandID)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		if syncErr := enqueueOrRun(brandID, socialsync.OpInboxSync); syncErr != nil {
			log.Printf("inbox: read-through enqueue failed for brand %s: %v", brandID, syncErr)
		}
	}
	q := filterToQuery(filter)
	if (q == trendlymodels.InboxQuery{}) {
		return trendlymodels.ListInboxConversations(brandID)
	}
	return trendlymodels.ListInboxConversationsFiltered(brandID, q)
}

// UnreadCount returns the number of unread conversations for a brand.
func UnreadCount(brandID string) int {
	n, err := trendlymodels.CountUnreadInboxConversations(brandID)
	if err != nil {
		return 0
	}
	return n
}

// SyncFromMeta pulls DM conversations for every connected account and upserts
// them into the store. Comments are populated via webhooks (see Phase 5), not here.
func SyncFromMeta(brandID string) error {
	socials, err := trendlymodels.ListBrandSocialAccounts(brandID)
	if err != nil {
		return err
	}
	var firstErr error
	for i := range socials {
		s := socials[i]
		if !isInboxChannel(s.Platform) {
			continue
		}
		tok, err := trendlymodels.GetBrandSocialToken(brandID, s.ID)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := syncAccountDMs(brandID, &s, tok.AccessToken); err != nil {
			log.Printf("inbox: DM sync failed for %s/%s: %v", brandID, s.ID, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// syncAccountDMs fetches recent DM conversations for one account and upserts them.
func syncAccountDMs(brandID string, s *trendlymodels.SocialAccount, token string) error {
	selfID := s.PlatformAccountID

	if s.Platform == trendlymodels.PlatformInstagram {
		data, err := instagram.GetIGConversations(token)
		if err != nil {
			return err
		}
		for ci := range data.Data {
			upsertDMConversation(brandID, s, selfID, token, &data.Data[ci])
		}
		return nil
	}

	// Facebook Page: list conversations, then hydrate recent messages per thread.
	convs, err := facebook.GetConversationsPaginated("", 25, token)
	if err != nil {
		return err
	}
	for ci := range convs.Data {
		conv := convs.Data[ci]
		msgs, err := facebook.GetMessagesWithPagination(conv.ID, "", 25, token)
		if err != nil {
			log.Printf("inbox: messages fetch failed for conv %s: %v", conv.ID, err)
			continue
		}
		conv.Messages = &struct {
			Data []facebook.Message `json:"data"`
		}{Data: msgs.Data}
		upsertDMConversation(brandID, s, selfID, token, &conv)
	}
	return nil
}

// isSelfParticipant reports whether a conversation participant is the connected
// account itself, rather than the external contact. Meta returns app-scoped ids
// (IGSID/PSID) in participants which don't always equal our stored
// PlatformAccountID, so we also match the linked IG business id and the account's
// own username — otherwise the business gets mis-picked as the contact and its
// page handle shows up as the sender name.
func isSelfParticipant(s *trendlymodels.SocialAccount, selfID, id, username string) bool {
	if id != "" {
		if id == selfID ||
			(s.PlatformAccountID != "" && id == s.PlatformAccountID) ||
			(s.InstagramBusinessID != "" && id == s.InstagramBusinessID) {
			return true
		}
	}
	if username != "" && s.Username != "" && strings.EqualFold(username, s.Username) {
		return true
	}
	return false
}

// mapMessengerMessage converts a Meta DM message to our InboxMessage, deciding
// direction with the robust self-detection (app-scoped from.id / username) and
// carrying any media attachment through (photos, videos, reels, shared posts,
// story replies, voice clips, files) so media-only messages don't render empty.
// Shared by the bulk sync and the unit-level thread/message resyncs.
func mapMessengerMessage(s *trendlymodels.SocialAccount, selfID string, m facebook.Message) trendlymodels.InboxMessage {
	author := trendlymodels.InboxAuthorContact
	if isSelfParticipant(s, selfID, m.From.ID, m.From.Username) {
		author = trendlymodels.InboxAuthorBusiness
	}
	msg := trendlymodels.InboxMessage{
		ID:     m.ID,
		Author: author,
		Text:   m.Message,
		SentAt: m.CreatedTime.UnixMilli(),
	}
	if media := m.FirstMedia(); media != nil {
		msg.AttachmentURL = media.URL
		msg.AttachmentType = media.Type
		msg.AttachmentThumbURL = media.Thumb
	}
	return msg
}

// fetchContactProfile looks up a DM contact's display profile (name, handle,
// avatar) from Meta. Best-effort: returns zero values on any error so callers
// fall back to whatever they already have. Instagram-Login accounts resolve via
// graph.instagram.com; Facebook Pages (incl. IG-via-Page) via graph.facebook.com.
//
// usernameHint seeds the Business Discovery fallback (the contact's handle from
// the conversation participants) for cases where the messaging lookup returns
// nothing — webhook callers that lack a participant list pass "".
func fetchContactProfile(s *trendlymodels.SocialAccount, token, contactID, usernameHint string) (name, handle, avatar string) {
	if contactID == "" || token == "" {
		log.Printf("inbox: contact profile fetch skipped account=%s contact=%q tokenSet=%v", s.ID, contactID, token != "")
		return "", "", ""
	}
	var (
		prof *facebook.UserProfile
		err  error
	)
	if s.Platform == trendlymodels.PlatformInstagram {
		prof, err = instagram.GetUser(contactID, token)
	} else {
		prof, err = facebook.GetUser(contactID, token)
	}
	if err != nil || prof == nil {
		// Best-effort BY DESIGN: a failure here — e.g. Meta withholds the profile,
		// or the Page token lacks pages_messaging / pages_read_engagement (code 190
		// "...must be granted before impersonating a user's page") — must NOT abort
		// webhook/sync processing. Callers keep going and upsert the message with
		// whatever they already have. We log platform/account/contact + the raw Meta
		// error so the permission/config issue is diagnosable without blocking ingestion.
		log.Printf("inbox: contact profile fetch failed (non-fatal) platform=%s account=%s contact=%s: %v", s.Platform, s.ID, contactID, err)
	} else {
		name, handle, avatar = prof.Name, prof.Username, prof.ProfilePic
		log.Printf("inbox: contact profile fetched platform=%s account=%s contact=%s name=%q handle=%q hasAvatar=%v", s.Platform, s.ID, contactID, name, handle, avatar != "")
	}
	if handle == "" {
		handle = usernameHint
	}

	// Business/professional accounts: the messaging User Profile API withholds
	// name + profile_pic (returns only the username), so the contact ends up with
	// the @handle and no avatar. Fall back to the Business Discovery API by
	// username, which exposes public profile data (name, picture) for professional
	// target accounts. Best-effort: failures (incl. personal accounts, which are
	// not discoverable) leave the messaging-API values untouched.
	if avatar == "" && handle != "" {
		if bd := fetchBusinessDiscovery(s, token, handle); bd != nil {
			if name == "" {
				name = bd.Name
			}
			if bd.ProfilePictureURL != "" {
				avatar = bd.ProfilePictureURL
			}
		}
	}
	log.Printf("inbox: contact profile resolved account=%s contact=%s name=%q handle=%q hasAvatar=%v", s.ID, contactID, name, handle, avatar != "")
	return name, handle, avatar
}

// fetchBusinessDiscovery resolves a professional contact's public profile by
// username via the Business Discovery API, keyed off the connected account's own
// IG id. Returns nil when no usable query node/token exists or the lookup fails.
func fetchBusinessDiscovery(s *trendlymodels.SocialAccount, token, username string) *facebook.InstagramProfile {
	var (
		prof *facebook.InstagramProfile
		err  error
	)
	if s.Platform == trendlymodels.PlatformInstagram {
		if s.PlatformAccountID == "" {
			return nil
		}
		prof, err = instagram.GetInstagramByUsername(s.PlatformAccountID, username, token)
	} else {
		if s.InstagramBusinessID == "" {
			return nil
		}
		prof, err = facebook.GetInstagramByUsername(s.InstagramBusinessID, username, token)
	}
	if err != nil || prof == nil {
		log.Printf("inbox: business discovery failed for %s: %v", username, err)
		return nil
	}
	return prof
}

// upsertDMConversation maps a Meta conversation to the inbox model and stores it.
func upsertDMConversation(brandID string, s *trendlymodels.SocialAccount, selfID, token string, c *facebook.ConversationMessagesData) {
	// Identify the contact participant (the one that isn't us).
	var contactID, contactName, contactHandle string
	for _, p := range c.Participants.Data {
		if isSelfParticipant(s, selfID, p.ID, p.Username) {
			continue
		}
		contactID = p.ID
		contactName = p.Username
		contactHandle = p.Username
		break
	}

	// Hydrate the contact's real display name + avatar from Meta. Participants
	// carry only an id/username and never a profile picture, so without this the
	// avatar is empty and the name is just the handle.
	var avatarURL string
	if name, handle, avatar := fetchContactProfile(s, token, contactID, contactHandle); name != "" || avatar != "" {
		if name != "" {
			contactName = name
		}
		if handle != "" {
			contactHandle = handle
		}
		avatarURL = avatar
	}

	msgs := make([]trendlymodels.InboxMessage, 0)
	var lastAt, lastInboundAt int64
	var preview string
	if c.Messages != nil {
		// Meta returns newest-first; reverse to chronological for the thread.
		data := c.Messages.Data
		for i := len(data) - 1; i >= 0; i-- {
			msg := mapMessengerMessage(s, selfID, data[i])
			msgs = append(msgs, msg)
			if msg.SentAt > lastAt {
				lastAt = msg.SentAt
				preview = inboxMsgPreview(msg)
			}
			if msg.Author == trendlymodels.InboxAuthorContact && msg.SentAt > lastInboundAt {
				lastInboundAt = msg.SentAt
			}
		}
	}

	// Deterministic conversation id per (account, contact). Meta's conversation
	// id (c.ID) is not available to webhook ingestion, so keying by the
	// participant pair lets fetch and webhook paths converge on the same doc.
	convID := "dm_" + s.ID + "_" + contactID

	// Preserve an existing last-seen baseline across resyncs so newly-fetched
	// inbound messages still surface as unread. On the FIRST sync (no existing
	// doc) baseline lastSeenAt to the latest activity so history starts read.
	lastSeenAt := lastAt
	if existing, err := trendlymodels.GetInboxConversation(brandID, convID); err == nil && existing != nil {
		lastSeenAt = existing.LastSeenAt
	}

	conv := &trendlymodels.InboxConversation{
		ID:      convID,
		Kind:    trendlymodels.InboxKindDM,
		Channel: s.Platform,
		Participant: trendlymodels.InboxParticipant{
			ID:        contactID,
			Name:      firstNonEmpty(contactName, "Unknown"),
			Handle:    contactHandle,
			AvatarURL: avatarURL,
		},
		Preview:                preview,
		LastActivityAt:         lastAt,
		LastSeenAt:             lastSeenAt,
		Unread:                 lastAt > lastSeenAt,
		Messages:               msgs,
		SocialID:               s.ID,
		ExternalConversationID: c.ID,
		ExternalRecipientID:    contactID,
		UpdatedAt:              time.Now().UnixMilli(),
	}
	if lastInboundAt > 0 {
		conv.ReplyWindowExpiresAt = lastInboundAt + replyWindowMs
	}
	if err := conv.Upsert(brandID); err != nil {
		log.Printf("inbox: upsert DM conv %s failed: %v", conv.ID, err)
	}
}

// ── Write operations ──────────────────────────────────────────────────────────

// Reply sends a DM reply or posts a public comment reply, then updates the store.
func Reply(brandID, convID, text string) error {
	conv, err := trendlymodels.GetInboxConversation(brandID, convID)
	if err != nil {
		return err
	}
	sa, err := loadServingAccount(brandID, conv.SocialID)
	if err != nil {
		return err
	}
	isFB := sa.account.Platform == trendlymodels.PlatformFacebook
	now := time.Now().UnixMilli()

	reply := trendlymodels.InboxMessage{
		ID:     fmt.Sprintf("local_%d", now),
		Author: trendlymodels.InboxAuthorBusiness,
		Text:   text,
		SentAt: now,
	}

	if conv.Kind == trendlymodels.InboxKindDM {
		// Enforce the 24h reply window server-side.
		if conv.ReplyWindowExpiresAt > 0 && now > conv.ReplyWindowExpiresAt {
			return fmt.Errorf("reply window expired")
		}
		// Capture the platform's returned message id (mid) and store it as the
		// reply's id, so the message_echoes webhook for this same send dedupes
		// against it (ingestMessagingForBrand matches by mid) instead of being
		// appended a second time.
		var sent *facebook.IMessageResponse
		if isFB {
			sent, err = facebook.SendTextMessage(conv.ExternalRecipientID, text, sa.token)
		} else {
			sent, err = instagram.SendIGMessage(conv.ExternalRecipientID, text, sa.token)
		}
		if err != nil {
			return err
		}
		if sent != nil && sent.MessageID != "" {
			reply.ID = sent.MessageID
		}
		conv.Messages = append(conv.Messages, reply)
		conv.Preview = text
		conv.LastActivityAt = now
		conv.LastSeenAt = now // replying implies the thread has been seen
		conv.Unread = false
		conv.UpdatedAt = now
		return conv.Upsert(brandID)
	}

	// Comment reply. Capture the platform's returned comment id and store it as
	// the reply's id, so the webhook echo of this same comment (and any Meta
	// redelivery) dedupes against it in ingestCommentReply instead of being
	// appended a second time. Comment webhooks carry no is_echo flag, so id-based
	// dedup is the only reliable guard.
	var newCommentID string
	if isFB {
		newCommentID, err = facebook.CreateCommentReply(conv.ExternalCommentID, text, sa.token)
	} else {
		newCommentID, err = instagram.ReplyToIGComment(conv.ExternalCommentID, text, sa.token)
	}
	if err != nil {
		return err
	}
	if newCommentID != "" {
		reply.ID = newCommentID
	}
	if conv.Comment != nil {
		conv.Comment.Replies = append(conv.Comment.Replies, reply)
	}
	conv.LastActivityAt = now
	conv.LastSeenAt = now // replying implies the thread has been seen
	conv.Unread = false
	conv.UpdatedAt = now
	return conv.Upsert(brandID)
}

// SetCommentHidden hides/unhides a comment on the platform and in the store.
func SetCommentHidden(brandID, convID string, hidden bool) error {
	conv, err := trendlymodels.GetInboxConversation(brandID, convID)
	if err != nil {
		return err
	}
	if conv.Kind != trendlymodels.InboxKindComment || conv.Comment == nil {
		return fmt.Errorf("conversation %s is not a comment", convID)
	}
	sa, err := loadServingAccount(brandID, conv.SocialID)
	if err != nil {
		return err
	}
	if sa.account.Platform == trendlymodels.PlatformFacebook {
		err = facebook.SetCommentHidden(conv.ExternalCommentID, hidden, sa.token)
	} else {
		err = instagram.SetIGCommentHidden(conv.ExternalCommentID, hidden, sa.token)
	}
	if err != nil {
		return err
	}
	return trendlymodels.UpdateInboxConversation(brandID, convID, []firestore.Update{
		{Path: "comment.hidden", Value: hidden},
		{Path: "updatedAt", Value: time.Now().UnixMilli()},
	})
}

// DeleteComment deletes a comment on the platform and removes it from the store.
func DeleteComment(brandID, convID string) error {
	conv, err := trendlymodels.GetInboxConversation(brandID, convID)
	if err != nil {
		return err
	}
	if conv.Kind != trendlymodels.InboxKindComment {
		return fmt.Errorf("conversation %s is not a comment", convID)
	}
	sa, err := loadServingAccount(brandID, conv.SocialID)
	if err != nil {
		return err
	}
	if sa.account.Platform == trendlymodels.PlatformFacebook {
		err = facebook.DeleteObject(conv.ExternalCommentID, sa.token)
	} else {
		err = instagram.DeleteIGObject(conv.ExternalCommentID, sa.token)
	}
	if err != nil {
		return err
	}
	return trendlymodels.DeleteInboxConversation(brandID, convID)
}

// MarkRead clears the unread flag on a conversation and advances lastSeenAt to
// the latest activity so the per-conversation new-message count drops to 0.
func MarkRead(brandID, convID string) error {
	conv, err := trendlymodels.GetInboxConversation(brandID, convID)
	if err != nil {
		return err
	}
	return trendlymodels.UpdateInboxConversation(brandID, convID, []firestore.Update{
		{Path: "unread", Value: false},
		{Path: "lastSeenAt", Value: conv.LastActivityAt},
		{Path: "updatedAt", Value: time.Now().UnixMilli()},
	})
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
