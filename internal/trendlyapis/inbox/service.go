package inbox

import (
	"fmt"
	"log"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/instagram"
	"github.com/idivarts/backend-sls/pkg/messenger"
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
		if syncErr := SyncFromMeta(brandID); syncErr != nil {
			log.Printf("inbox: read-through sync failed for brand %s: %v", brandID, syncErr)
		}
	}
	return trendlymodels.ListInboxConversations(brandID)
}

// ListConversations returns the cached conversations without triggering a sync.
func ListConversations(brandID string) ([]trendlymodels.InboxConversation, error) {
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
		if syncErr := SyncFromMeta(brandID); syncErr != nil {
			log.Printf("inbox: read-through sync failed for brand %s: %v", brandID, syncErr)
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
			upsertDMConversation(brandID, s, selfID, &data.Data[ci])
		}
		return nil
	}

	// Facebook Page: list conversations, then hydrate recent messages per thread.
	convs, err := messenger.GetConversationsPaginated("", 25, token)
	if err != nil {
		return err
	}
	for ci := range convs.Data {
		conv := convs.Data[ci]
		msgs, err := messenger.GetMessagesWithPagination(conv.ID, "", 25, token)
		if err != nil {
			log.Printf("inbox: messages fetch failed for conv %s: %v", conv.ID, err)
			continue
		}
		conv.Messages = &struct {
			Data []messenger.Message `json:"data"`
		}{Data: msgs.Data}
		upsertDMConversation(brandID, s, selfID, &conv)
	}
	return nil
}

// upsertDMConversation maps a Meta conversation to the inbox model and stores it.
func upsertDMConversation(brandID string, s *trendlymodels.SocialAccount, selfID string, c *messenger.ConversationMessagesData) {
	// Identify the contact participant (the one that isn't us).
	var contactID, contactName, contactHandle string
	for _, p := range c.Participants.Data {
		if p.ID != selfID {
			contactID = p.ID
			contactName = p.Username
			contactHandle = p.Username
			break
		}
	}

	msgs := make([]trendlymodels.InboxMessage, 0)
	var lastAt, lastInboundAt int64
	var preview string
	if c.Messages != nil {
		// Meta returns newest-first; reverse to chronological for the thread.
		data := c.Messages.Data
		for i := len(data) - 1; i >= 0; i-- {
			m := data[i]
			author := trendlymodels.InboxAuthorContact
			if m.From.ID == selfID {
				author = trendlymodels.InboxAuthorBusiness
			}
			ts := m.CreatedTime.UnixMilli()
			msgs = append(msgs, trendlymodels.InboxMessage{
				ID:     m.ID,
				Author: author,
				Text:   m.Message,
				SentAt: ts,
			})
			if ts > lastAt {
				lastAt = ts
				preview = m.Message
			}
			if author == trendlymodels.InboxAuthorContact && ts > lastInboundAt {
				lastInboundAt = ts
			}
		}
	}

	conv := &trendlymodels.InboxConversation{
		ID:      "dm_" + c.ID,
		Kind:    trendlymodels.InboxKindDM,
		Channel: s.Platform,
		Participant: trendlymodels.InboxParticipant{
			ID:     contactID,
			Name:   firstNonEmpty(contactName, "Unknown"),
			Handle: contactHandle,
		},
		Preview:                preview,
		LastActivityAt:         lastAt,
		Unread:                 false,
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
		if isFB {
			if _, err := messenger.SendTextMessage(conv.ExternalRecipientID, text, sa.token); err != nil {
				return err
			}
		} else {
			if _, err := instagram.SendIGMessage(conv.ExternalRecipientID, text, sa.token); err != nil {
				return err
			}
		}
		conv.Messages = append(conv.Messages, reply)
		conv.Preview = text
		conv.LastActivityAt = now
		conv.Unread = false
		conv.UpdatedAt = now
		return conv.Upsert(brandID)
	}

	// Comment reply.
	if isFB {
		if _, err := messenger.CreateCommentReply(conv.ExternalCommentID, text, sa.token); err != nil {
			return err
		}
	} else {
		if _, err := instagram.ReplyToIGComment(conv.ExternalCommentID, text, sa.token); err != nil {
			return err
		}
	}
	if conv.Comment != nil {
		conv.Comment.Replies = append(conv.Comment.Replies, reply)
	}
	conv.LastActivityAt = now
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
		err = messenger.SetCommentHidden(conv.ExternalCommentID, hidden, sa.token)
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
		err = messenger.DeleteObject(conv.ExternalCommentID, sa.token)
	} else {
		err = instagram.DeleteIGObject(conv.ExternalCommentID, sa.token)
	}
	if err != nil {
		return err
	}
	return trendlymodels.DeleteInboxConversation(brandID, convID)
}

// MarkRead clears the unread flag on a conversation.
func MarkRead(brandID, convID string) error {
	return trendlymodels.UpdateInboxConversation(brandID, convID, []firestore.Update{
		{Path: "unread", Value: false},
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
