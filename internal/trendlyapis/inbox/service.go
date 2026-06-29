package inbox

import (
	"fmt"
	"log"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/idivarts/backend-sls/internal/constants"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/internal/socialsync"
	"github.com/idivarts/backend-sls/pkg/facebook"
	"github.com/idivarts/backend-sls/pkg/instagram"
	"github.com/idivarts/backend-sls/pkg/reddit"
	"github.com/idivarts/backend-sls/pkg/twitter"
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

// isDMChannel reports whether a platform supports the direct-message (Messages)
// inbox. Meta (IG/FB) sync via webhooks + read-through; Twitter via polling;
// Reddit via read-only PM polling. LinkedIn is intentionally EXCLUDED — LinkedIn
// exposes no company-page messaging API (closed partner program), so it must
// never appear as a DM channel (see the "Linkedin Messaging" ticket).
func isDMChannel(p trendlymodels.Platform) bool {
	switch p {
	case trendlymodels.PlatformInstagram, trendlymodels.PlatformFacebook,
		trendlymodels.PlatformTwitter:
		return true
	case trendlymodels.PlatformReddit:
		return constants.RedditEnabled // gated — see internal/constants/features.go
	}
	return false
}

// isCommentChannel reports whether a platform supports the comments (Media)
// inbox. LinkedIn COMPANY PAGES (linkedin_page) have org-post comments via the
// CMA; personal LinkedIn does NOT (no API), so it is excluded.
func isCommentChannel(p trendlymodels.Platform) bool {
	switch p {
	case trendlymodels.PlatformInstagram, trendlymodels.PlatformFacebook,
		trendlymodels.PlatformLinkedInPage, trendlymodels.PlatformTwitter:
		return true
	case trendlymodels.PlatformReddit:
		return constants.RedditEnabled // gated — see internal/constants/features.go
	}
	return false
}

// isInboxChannel reports whether a platform participates in the inbox in ANY
// capacity (DM or comments). Used to surface connected accounts.
func isInboxChannel(p trendlymodels.Platform) bool {
	return isDMChannel(p) || isCommentChannel(p)
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
	tok, err := trendlymodels.GetBrandSocialTokenForAccount(brandID, acc)
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
		if !isDMChannel(s.Platform) {
			continue
		}
		tok, err := trendlymodels.GetBrandSocialTokenForAccount(brandID, &s)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		var serr error
		switch s.Platform {
		case trendlymodels.PlatformTwitter:
			serr = syncTwitterDMs(brandID, &s, tok.AccessToken)
		case trendlymodels.PlatformReddit:
			serr = syncRedditPMs(brandID, &s, tok.AccessToken)
		default: // instagram / facebook
			serr = syncAccountDMs(brandID, &s, tok.AccessToken)
		}
		if serr != nil {
			log.Printf("inbox: DM sync failed for %s/%s (%s): %v", brandID, s.ID, s.Platform, serr)
			if firstErr == nil {
				firstErr = serr
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
	convs, err := facebook.GetConversationsPaginated("", 25, token, facebook.PlatformMessenger)
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
// fall back to whatever they already have.
//
// The right API depends on the CONTACT's platform, not just the connected
// account's: a Facebook Page serves BOTH Messenger contacts (PSIDs) and IG
// contacts (IGSIDs, via a linked IG Business account). The `channel` of the
// event disambiguates them:
//   - facebook → Messenger User Profile API (first_name/last_name/profile_pic;
//     FB users have no username/follower_count — requesting those 400s).
//   - instagram → IG messaging User Profile API (name/username/profile_pic/…),
//     via graph.instagram.com for IG-Login accounts or graph.facebook.com for
//     IG-via-Page, plus a Business Discovery fallback by username.
//
// usernameHint seeds the IG Business Discovery fallback (the contact's handle
// from the conversation participants); webhook callers that lack one pass "".
func fetchContactProfile(s *trendlymodels.SocialAccount, channel trendlymodels.Platform, token, contactID, usernameHint string) (name, handle, avatar string) {
	if contactID == "" || token == "" {
		log.Printf("inbox: contact profile fetch skipped account=%s contact=%q tokenSet=%v", s.ID, contactID, token != "")
		return "", "", ""
	}

	// ── Facebook (Messenger) contact: PSID, no username/follower_count. ──
	if channel == trendlymodels.PlatformFacebook {
		prof, err := facebook.GetMessengerUser(contactID, token)
		if err != nil || prof == nil {
			// Best-effort BY DESIGN: a failure (Meta withholds the profile, or the
			// page token lacks pages_messaging) must NOT abort ingestion.
			log.Printf("inbox: contact profile fetch failed (non-fatal) channel=facebook account=%s contact=%s: %v", s.ID, contactID, err)
			return "", "", ""
		}
		name, avatar = prof.FullName(), prof.ProfilePic
		log.Printf("inbox: contact profile fetched channel=facebook account=%s contact=%s name=%q hasAvatar=%v", s.ID, contactID, name, avatar != "")
		return name, "", avatar
	}

	// ── Instagram contact: IGSID via the IG messaging User Profile API. ──
	var (
		prof *facebook.UserProfile
		err  error
	)
	if s.Platform == trendlymodels.PlatformInstagram {
		prof, err = instagram.GetUser(contactID, token) // graph.instagram.com
	} else {
		prof, err = facebook.GetUser(contactID, token) // IG-via-Page, graph.facebook.com
	}
	if err != nil || prof == nil {
		log.Printf("inbox: contact profile fetch failed (non-fatal) channel=instagram account=%s contact=%s: %v", s.ID, contactID, err)
	} else {
		name, handle, avatar = prof.Name, prof.Username, prof.ProfilePic
		log.Printf("inbox: contact profile fetched channel=instagram account=%s contact=%s name=%q handle=%q hasAvatar=%v", s.ID, contactID, name, handle, avatar != "")
	}
	if handle == "" {
		handle = usernameHint
	}

	// Professional IG accounts: the messaging API withholds name + profile_pic
	// (only the username), so fall back to Business Discovery by username for
	// public profile data. Best-effort; personal accounts aren't discoverable.
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
	if name, handle, avatar := fetchContactProfile(s, s.Platform, token, contactID, contactHandle); name != "" || avatar != "" {
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
		// Enforce the 24h reply window server-side (Meta only; non-Meta channels
		// leave ReplyWindowExpiresAt=0 so this never triggers).
		if conv.ReplyWindowExpiresAt > 0 && now > conv.ReplyWindowExpiresAt {
			return fmt.Errorf("reply window expired")
		}
		// Capture the platform's returned message id and store it as the reply's
		// id, so the echo webhook (Meta) dedupes against it instead of appending
		// a second copy.
		switch sa.account.Platform {
		case trendlymodels.PlatformFacebook:
			var sent *facebook.IMessageResponse
			sent, err = facebook.SendTextMessage(conv.ExternalRecipientID, text, sa.token)
			if err == nil && sent != nil && sent.MessageID != "" {
				reply.ID = sent.MessageID
			}
		case trendlymodels.PlatformInstagram:
			var sent *facebook.IMessageResponse
			sent, err = instagram.SendIGMessage(conv.ExternalRecipientID, text, sa.token)
			if err == nil && sent != nil && sent.MessageID != "" {
				reply.ID = sent.MessageID
			}
		case trendlymodels.PlatformTwitter:
			var dmID string
			dmID, err = twitter.SendDM(sa.token, conv.ExternalRecipientID, text)
			if err == nil && dmID != "" {
				reply.ID = dmID
			}
		case trendlymodels.PlatformReddit:
			// Reddit PMs are read-only since Aug 2025; compose is best-effort and
			// usually 403s. We surface the API error to the caller.
			err = reddit.ComposeMessage(sa.token, conv.ExternalRecipientID, "Re: "+conv.Preview, text)
		default:
			err = fmt.Errorf("messaging not supported for platform %q", sa.account.Platform)
		}
		if err != nil {
			return err
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
