package inbox

import (
	"log"
	"sort"
	"strings"
	"time"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/reddit"
	"github.com/idivarts/backend-sls/pkg/twitter"
)

// neutralContact is a platform-agnostic DM contact (the external party).
type neutralContact struct {
	ID        string
	Name      string
	Handle    string
	AvatarURL string
}

// upsertNeutralDMConversation stores a DM thread for a NON-Meta platform
// (Twitter, Reddit). It mirrors the tail of upsertDMConversation but takes
// already-normalized messages + contact instead of a Meta payload. messages
// should be in chronological order. replyWindowExpiresAt=0 means "no window"
// (only Meta enforces the 24h rule).
func upsertNeutralDMConversation(brandID string, s *trendlymodels.SocialAccount, contact neutralContact, msgs []trendlymodels.InboxMessage, replyWindowExpiresAt int64, externalConvID string) {
	if contact.ID == "" {
		return
	}
	var lastAt, lastInboundAt int64
	var preview string
	for _, m := range msgs {
		if m.SentAt > lastAt {
			lastAt = m.SentAt
			preview = inboxMsgPreview(m)
		}
		if m.Author == trendlymodels.InboxAuthorContact && m.SentAt > lastInboundAt {
			lastInboundAt = m.SentAt
		}
	}

	convID := "dm_" + s.ID + "_" + contact.ID

	lastSeenAt := lastAt
	if existing, err := trendlymodels.GetInboxConversation(brandID, convID); err == nil && existing != nil {
		lastSeenAt = existing.LastSeenAt
	}

	conv := &trendlymodels.InboxConversation{
		ID:      convID,
		Kind:    trendlymodels.InboxKindDM,
		Channel: s.Platform,
		Participant: trendlymodels.InboxParticipant{
			ID:        contact.ID,
			Name:      firstNonEmpty(contact.Name, contact.Handle, "Unknown"),
			Handle:    contact.Handle,
			AvatarURL: contact.AvatarURL,
		},
		Preview:                preview,
		LastActivityAt:         lastAt,
		LastSeenAt:             lastSeenAt,
		Unread:                 lastAt > lastSeenAt,
		Messages:               msgs,
		SocialID:               s.ID,
		ExternalConversationID: externalConvID,
		ExternalRecipientID:    contact.ID,
		UpdatedAt:              time.Now().UnixMilli(),
	}
	if replyWindowExpiresAt > 0 {
		conv.ReplyWindowExpiresAt = replyWindowExpiresAt
	}
	if err := conv.Upsert(brandID); err != nil {
		log.Printf("inbox: upsert neutral DM conv %s failed: %v", conv.ID, err)
	}
}

// syncTwitterDMs polls the connected X account's DM events and upserts them,
// grouped by conversation. X has no free webhook so this is the only delivery
// path; respect the per-account 15-calls/15min cap upstream.
func syncTwitterDMs(brandID string, s *trendlymodels.SocialAccount, token string) error {
	selfID, _ := s.RawProfile["id"].(string)

	events, err := twitter.GetDMEvents(token, 100)
	if err != nil {
		return err
	}

	type bucket struct {
		msgs      []trendlymodels.InboxMessage
		contactID string
	}
	convs := map[string]*bucket{}
	for _, e := range events {
		author := trendlymodels.InboxAuthorContact
		if selfID != "" && e.SenderID == selfID {
			author = trendlymodels.InboxAuthorBusiness
		}
		b := convs[e.DMConversationID]
		if b == nil {
			b = &bucket{}
			convs[e.DMConversationID] = b
		}
		b.msgs = append(b.msgs, trendlymodels.InboxMessage{
			ID:     e.ID,
			Author: author,
			Text:   e.Text,
			SentAt: e.CreatedAt.UnixMilli(),
		})
		if author == trendlymodels.InboxAuthorContact && b.contactID == "" {
			b.contactID = e.SenderID
		}
	}

	for convID, b := range convs {
		// Can't surface a thread with no identifiable external participant.
		if b.contactID == "" {
			continue
		}
		sort.SliceStable(b.msgs, func(i, j int) bool { return b.msgs[i].SentAt < b.msgs[j].SentAt })
		contact := neutralContact{ID: b.contactID}
		if u, uerr := twitter.GetUserByID(token, b.contactID); uerr == nil && u != nil {
			contact.Name = u.Name
			contact.Handle = u.Username
			contact.AvatarURL = strings.Replace(u.ProfileImageURL, "_normal", "", 1)
		}
		upsertNeutralDMConversation(brandID, s, contact, b.msgs, 0, convID)
	}
	return nil
}

// syncRedditPMs polls the connected Reddit account's private-message inbox and
// upserts threads grouped by the other party. Reddit PMs are READ-ONLY since
// Aug 2025 (replies are best-effort, see Reply()); this surfaces them so brands
// can at least see inbound messages.
func syncRedditPMs(brandID string, s *trendlymodels.SocialAccount, token string) error {
	msgs, err := reddit.GetInbox(token, 50)
	if err != nil {
		return err
	}

	self := s.Username
	type bucket struct {
		msgs []trendlymodels.InboxMessage
	}
	convs := map[string]*bucket{}
	for _, m := range msgs {
		// The contact is whichever party isn't us.
		contactName := m.Author
		author := trendlymodels.InboxAuthorContact
		if strings.EqualFold(m.Author, self) {
			contactName = m.Dest
			author = trendlymodels.InboxAuthorBusiness
		}
		if contactName == "" {
			continue
		}
		b := convs[contactName]
		if b == nil {
			b = &bucket{}
			convs[contactName] = b
		}
		b.msgs = append(b.msgs, trendlymodels.InboxMessage{
			ID:     m.Fullname,
			Author: author,
			Text:   strings.TrimSpace(m.Subject + "\n" + m.Body),
			SentAt: m.CreatedUTC * 1000,
		})
	}

	for contactName, b := range convs {
		sort.SliceStable(b.msgs, func(i, j int) bool { return b.msgs[i].SentAt < b.msgs[j].SentAt })
		contact := neutralContact{ID: contactName, Name: contactName, Handle: contactName}
		upsertNeutralDMConversation(brandID, s, contact, b.msgs, 0, "")
	}
	return nil
}
