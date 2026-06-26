package inbox

import (
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/instagram"
	"github.com/idivarts/backend-sls/pkg/facebook"
)

// Media-tab support. Unlike conversations (Firestore-backed, webhook-fed), the
// Media tab reads on demand from the Graph API so we never bulk-store every
// historical comment. A brand browses its published posts/reels, taps one, and
// we fetch that media's comments live. Replies / hide / delete reuse the same
// platform calls as the conversation actions, keyed by the comment id.

const (
	defaultMediaCount   = 12
	defaultCommentCount = 25
)

// MediaItem is a single published post/reel across a connected account.
type MediaItem struct {
	ID            string `json:"id"`
	Channel       string `json:"channel"`  // "instagram" | "facebook"
	SocialID      string `json:"socialId"` // serving SocialAccount.ID
	ThumbnailURL  string `json:"thumbnailUrl,omitempty"`
	Caption       string `json:"caption,omitempty"`
	Permalink     string `json:"permalink,omitempty"`
	Timestamp     int64  `json:"timestamp"` // epoch ms
	CommentsCount int    `json:"commentsCount"`
	LikeCount     int    `json:"likeCount"`
}

// MediaComment is a top-level comment on a piece of media.
type MediaComment struct {
	ID        string                         `json:"id"`
	Channel   string                         `json:"channel"`
	Author    trendlymodels.InboxParticipant `json:"author"`
	Text      string                         `json:"text"`
	Timestamp int64                          `json:"timestamp"` // epoch ms (0 if unknown)
}

// graphTypeForIG returns the GetMedia/GetComments GraphType for an IG read on a
// given account: directly-connected IG accounts use the Instagram Graph (non-zero);
// IG Business accounts linked to a Facebook Page are read via the Facebook Graph (0).
func graphTypeForIG(acc *trendlymodels.SocialAccount) int {
	if acc.Platform == trendlymodels.PlatformInstagram {
		return 1
	}
	return 0
}

// igMediaItem maps an Instagram media node to the normalized MediaItem.
func igMediaItem(m instagram.InstagramMedia, socialID string) MediaItem {
	thumb := m.ThumbnailURL
	if thumb == "" {
		thumb = m.MediaURL
	}
	return MediaItem{
		ID:            m.ID,
		Channel:       trendlymodels.PlatformInstagram,
		SocialID:      socialID,
		ThumbnailURL:  thumb,
		Caption:       m.Caption,
		Permalink:     m.Permalink,
		Timestamp:     m.Timestamp.UnixMilli(),
		CommentsCount: m.CommentsCount,
		LikeCount:     m.LikeCount,
	}
}

// ListMedia returns the brand's published posts/reels across all connected Meta
// accounts, newest first. Best-effort: a per-account fetch failure is logged and
// skipped rather than failing the whole list.
func ListMedia(brandID string, count int) ([]MediaItem, error) {
	if count <= 0 {
		count = defaultMediaCount
	}
	socials, err := trendlymodels.ListBrandSocialAccounts(brandID)
	if err != nil {
		return nil, err
	}

	out := make([]MediaItem, 0)
	var firstErr error
	record := func(e error) {
		if e != nil && firstErr == nil {
			firstErr = e
		}
	}

	for i := range socials {
		s := socials[i]
		if !isInboxChannel(s.Platform) {
			continue
		}
		tok, err := trendlymodels.GetBrandSocialToken(brandID, s.ID)
		if err != nil {
			record(err)
			continue
		}
		token := tok.AccessToken

		if s.Platform == trendlymodels.PlatformFacebook {
			// Facebook Page posts.
			posts, err := facebook.GetPosts(s.PlatformAccountID, token, facebook.IFBPostsParams{Count: count})
			if err != nil {
				log.Printf("inbox media: FB posts fetch failed for %s/%s: %v", brandID, s.ID, err)
				record(err)
			}
			for _, p := range posts {
				out = append(out, MediaItem{
					ID:           p.ID,
					Channel:      trendlymodels.PlatformFacebook,
					SocialID:     s.ID,
					ThumbnailURL: p.FullPicture,
					Caption:      p.Message,
					Permalink:    p.PermalinkURL,
					Timestamp:    p.CreatedTime.UnixMilli(),
				})
			}
			// Linked IG Business account media (read via the Facebook Graph).
			if s.InstagramBusinessID != "" {
				media, err := instagram.GetMedia(s.InstagramBusinessID, token, instagram.IGetMediaParams{
					GraphType: 0,
					PageID:    s.InstagramBusinessID,
					Count:     count,
				})
				if err != nil {
					log.Printf("inbox media: linked-IG media fetch failed for %s/%s: %v", brandID, s.ID, err)
					record(err)
				}
				for _, m := range media {
					out = append(out, igMediaItem(m, s.ID))
				}
			}
			continue
		}

		// Directly-connected Instagram account (Instagram Graph).
		media, err := instagram.GetMedia(s.PlatformAccountID, token, instagram.IGetMediaParams{
			GraphType: 1,
			Count:     count,
		})
		if err != nil {
			log.Printf("inbox media: IG media fetch failed for %s/%s: %v", brandID, s.ID, err)
			record(err)
		}
		for _, m := range media {
			out = append(out, igMediaItem(m, s.ID))
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp > out[j].Timestamp })
	return out, firstErr
}

// RefreshMedia pulls the brand's published media from Meta and upserts each item
// to Firestore (brands/{brandId}/inboxMedia), so the Media tab observes them live
// as they're written. Runs in the social_sqs worker off the HTTP request path.
// Best-effort: a per-account Graph failure is logged and skipped.
func RefreshMedia(brandID string) error {
	items, err := ListMedia(brandID, defaultMediaCount)
	if err != nil {
		log.Printf("inbox media: refresh list partial/failed for %s: %v", brandID, err)
	}
	now := time.Now().UnixMilli()
	for i := range items {
		it := items[i]
		doc := &trendlymodels.InboxMediaDoc{
			ID:            it.ID,
			Channel:       it.Channel,
			SocialID:      it.SocialID,
			ThumbnailURL:  it.ThumbnailURL,
			Caption:       it.Caption,
			Permalink:     it.Permalink,
			Timestamp:     it.Timestamp,
			CommentsCount: it.CommentsCount,
			LikeCount:     it.LikeCount,
			UpdatedAt:     now,
		}
		if e := doc.Upsert(brandID); e != nil {
			log.Printf("inbox media: upsert %s failed for %s: %v", it.ID, brandID, e)
		}
	}
	return err
}

// ListMediaComments fetches the top-level comments on a single piece of media.
func ListMediaComments(brandID, socialID, channel, mediaID string, count int) ([]MediaComment, error) {
	if count <= 0 {
		count = defaultCommentCount
	}
	sa, err := loadServingAccount(brandID, socialID)
	if err != nil {
		return nil, err
	}

	out := make([]MediaComment, 0)
	if channel == trendlymodels.PlatformFacebook {
		items, err := facebook.GetCommentReplies(mediaID, sa.token)
		if err != nil {
			return nil, err
		}
		for _, it := range items {
			out = append(out, MediaComment{
				ID:      it.ID,
				Channel: trendlymodels.PlatformFacebook,
				Author: trendlymodels.InboxParticipant{
					ID:   it.From.ID,
					Name: firstNonEmpty(it.From.Name, "Unknown"),
					// Facebook users have no username/handle.
				},
				Text: it.Message,
			})
		}
		return out, nil
	}

	// Instagram comments.
	items, err := instagram.GetComments(mediaID, sa.token, instagram.IGetCommentsParams{
		GraphType: graphTypeForIG(sa.account),
		Count:     count,
	})
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		out = append(out, MediaComment{
			ID:      it.ID,
			Channel: trendlymodels.PlatformInstagram,
			Author: trendlymodels.InboxParticipant{
				ID:     it.From.ID,
				Name:   firstNonEmpty(it.From.Username, "Unknown"),
				Handle: it.From.Username,
			},
			Text:      it.Text,
			Timestamp: it.Timestamp.UnixMilli(),
		})
	}
	return out, nil
}

// ── Media-sourced comment actions (keyed by comment id, no stored conversation) ──

// loadCommentServing resolves the token + platform for a media-sourced comment action.
func loadCommentServing(brandID, socialID string) (*servingAccount, error) {
	return loadServingAccount(brandID, socialID)
}

// ReplyToMediaComment posts a public reply to a comment surfaced from the Media tab.
func ReplyToMediaComment(brandID, socialID, channel, commentID, text string) error {
	sa, err := loadCommentServing(brandID, socialID)
	if err != nil {
		return err
	}
	if channel == trendlymodels.PlatformFacebook {
		_, err = facebook.CreateCommentReply(commentID, text, sa.token)
		return err
	}
	_, err = instagram.ReplyToIGComment(commentID, text, sa.token)
	return err
}

// SetMediaCommentHidden hides/unhides a comment surfaced from the Media tab.
func SetMediaCommentHidden(brandID, socialID, channel, commentID string, hidden bool) error {
	sa, err := loadCommentServing(brandID, socialID)
	if err != nil {
		return err
	}
	if channel == trendlymodels.PlatformFacebook {
		return facebook.SetCommentHidden(commentID, hidden, sa.token)
	}
	return instagram.SetIGCommentHidden(commentID, hidden, sa.token)
}

// DeleteMediaComment deletes a comment surfaced from the Media tab.
func DeleteMediaComment(brandID, socialID, channel, commentID string) error {
	sa, err := loadCommentServing(brandID, socialID)
	if err != nil {
		return err
	}
	if channel == trendlymodels.PlatformFacebook {
		return facebook.DeleteObject(commentID, sa.token)
	}
	return instagram.DeleteIGObject(commentID, sa.token)
}

// channelOrDefault validates a channel string, defaulting to instagram.
func channelOrDefault(ch string) (string, error) {
	switch ch {
	case trendlymodels.PlatformInstagram, trendlymodels.PlatformFacebook:
		return ch, nil
	case "":
		return trendlymodels.PlatformInstagram, nil
	default:
		return "", fmt.Errorf("unsupported channel %q", ch)
	}
}
