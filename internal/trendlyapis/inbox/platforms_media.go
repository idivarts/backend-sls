package inbox

import (
	"log"
	"strings"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/linkedin"
	"github.com/idivarts/backend-sls/pkg/reddit"
	"github.com/idivarts/backend-sls/pkg/twitter"
)

// listPlatformMedia returns published media for a NON-Meta platform, mapping
// each platform's "posts" to the shared MediaItem. Best-effort: returns (nil,nil)
// when the platform can't serve media for this connection (e.g. a LinkedIn
// member-only connection with no administered org).
func listPlatformMedia(s *trendlymodels.SocialAccount, token string, count int) ([]MediaItem, error) {
	switch s.Platform {
	case trendlymodels.PlatformLinkedInPage:
		orgURN := linkedinOrgURN(s)
		if orgURN == "" {
			return nil, nil
		}
		posts, err := linkedin.ListOrgPosts(token, orgURN, count)
		if err != nil {
			return nil, err
		}
		out := make([]MediaItem, 0, len(posts))
		for _, p := range posts {
			out = append(out, MediaItem{
				ID:            p.URN,
				Channel:       trendlymodels.PlatformLinkedInPage,
				SocialID:      s.ID,
				ThumbnailURL:  p.ThumbnailURL,
				Caption:       p.Text,
				Permalink:     p.Permalink,
				Timestamp:     p.CreatedAt,
				CommentsCount: int(p.CommentCount),
				LikeCount:     int(p.LikeCount),
			})
		}
		return out, nil

	case trendlymodels.PlatformTwitter:
		selfID, _ := s.RawProfile["id"].(string)
		if selfID == "" {
			return nil, nil
		}
		tweets, err := twitter.GetUserTweets(token, selfID, count)
		if err != nil {
			return nil, err
		}
		out := make([]MediaItem, 0, len(tweets))
		for _, t := range tweets {
			out = append(out, MediaItem{
				ID:            t.ID,
				Channel:       trendlymodels.PlatformTwitter,
				SocialID:      s.ID,
				Caption:       t.Text,
				Permalink:     "https://twitter.com/" + s.Username + "/status/" + t.ID,
				Timestamp:     t.CreatedAt.UnixMilli(),
				CommentsCount: int(t.Replies),
				LikeCount:     int(t.Likes),
			})
		}
		return out, nil

	case trendlymodels.PlatformReddit:
		subs, err := reddit.GetUserSubmissions(token, s.Username, count)
		if err != nil {
			return nil, err
		}
		out := make([]MediaItem, 0, len(subs))
		for _, sub := range subs {
			thumb := sub.Thumbnail
			if thumb == "self" || thumb == "default" || thumb == "nsfw" || thumb == "spoiler" {
				thumb = ""
			}
			out = append(out, MediaItem{
				ID:            sub.ID, // bare id (t3 without prefix) — comment fetch uses this
				Channel:       trendlymodels.PlatformReddit,
				SocialID:      s.ID,
				ThumbnailURL:  thumb,
				Caption:       sub.Title,
				Permalink:     sub.Permalink,
				Timestamp:     sub.CreatedUTC * 1000,
				CommentsCount: int(sub.NumComments),
				LikeCount:     int(sub.Score),
			})
		}
		return out, nil
	}
	return nil, nil
}

// listPlatformComments fetches top-level comments for a NON-Meta media item.
//   - LinkedIn: mediaID is the post/share URN.
//   - Twitter:  mediaID is the tweet id (= conversation_id); the original tweet
//     itself is excluded from the returned replies.
//   - Reddit:   mediaID is the submission id (t3 without the prefix).
func listPlatformComments(sa *servingAccount, channel, mediaID string) ([]MediaComment, error) {
	switch channel {
	case trendlymodels.PlatformLinkedInPage:
		items, err := linkedin.GetComments(sa.token, mediaID)
		if err != nil {
			return nil, err
		}
		out := make([]MediaComment, 0, len(items))
		for _, c := range items {
			out = append(out, MediaComment{
				ID:      c.URN,
				Channel: trendlymodels.PlatformLinkedInPage,
				Author: trendlymodels.InboxParticipant{
					ID:        c.ActorURN,
					Name:      firstNonEmpty(c.ActorName, "LinkedIn member"),
					AvatarURL: c.ActorAvatar,
				},
				Text:      c.Text,
				Timestamp: c.CreatedAt,
			})
		}
		return out, nil

	case trendlymodels.PlatformTwitter:
		items, err := twitter.GetReplies(sa.token, mediaID)
		if err != nil {
			return nil, err
		}
		out := make([]MediaComment, 0, len(items))
		for _, r := range items {
			if r.ID == mediaID {
				continue // the root tweet, not a reply
			}
			out = append(out, MediaComment{
				ID:      r.ID,
				Channel: trendlymodels.PlatformTwitter,
				Author: trendlymodels.InboxParticipant{
					ID:        r.AuthorID,
					Name:      firstNonEmpty(r.AuthorName, r.AuthorUsername, "Unknown"),
					Handle:    r.AuthorUsername,
					AvatarURL: r.AuthorAvatar,
				},
				Text:      r.Text,
				Timestamp: r.CreatedAt.UnixMilli(),
			})
		}
		return out, nil

	case trendlymodels.PlatformReddit:
		items, err := reddit.GetComments(sa.token, mediaID)
		if err != nil {
			return nil, err
		}
		out := make([]MediaComment, 0, len(items))
		for _, c := range items {
			out = append(out, MediaComment{
				ID:      c.Fullname,
				Channel: trendlymodels.PlatformReddit,
				Author: trendlymodels.InboxParticipant{
					ID:     c.AuthorFullname,
					Name:   firstNonEmpty(c.Author, "Unknown"),
					Handle: c.Author,
				},
				Text:      c.Body,
				Timestamp: c.CreatedUTC * 1000,
			})
		}
		return out, nil
	}
	return nil, nil
}

// replyPlatformComment posts a reply to a NON-Meta media comment.
func replyPlatformComment(sa *servingAccount, channel, commentID, text string) error {
	switch channel {
	case trendlymodels.PlatformLinkedInPage:
		orgURN := linkedinOrgURN(sa.account)
		objectURN := linkedinObjectFromComment(commentID)
		if objectURN == "" {
			return errUnsupported("linkedin reply: could not derive post urn from comment")
		}
		_, err := linkedin.CreateCommentReply(sa.token, orgURN, objectURN, commentID, text)
		return err
	case trendlymodels.PlatformTwitter:
		_, err := twitter.ReplyToTweet(sa.token, commentID, text)
		return err
	case trendlymodels.PlatformReddit:
		_, err := reddit.ReplyToComment(sa.token, commentID, text)
		return err
	}
	return errUnsupported("reply not supported for channel " + channel)
}

// deletePlatformComment deletes a NON-Meta media comment (own content only).
func deletePlatformComment(sa *servingAccount, channel, commentID string) error {
	switch channel {
	case trendlymodels.PlatformLinkedInPage:
		orgURN := linkedinOrgURN(sa.account)
		return linkedin.DeleteComment(sa.token, orgURN, commentID)
	case trendlymodels.PlatformTwitter:
		return twitter.DeleteTweet(sa.token, commentID)
	case trendlymodels.PlatformReddit:
		return reddit.DeleteThing(sa.token, commentID)
	}
	return errUnsupported("delete not supported for channel " + channel)
}

// linkedinOrgURN derives a LinkedIn Page account's organization URN from its
// stored orgUrn (preferred) or its PlatformAccountID (the numeric org id).
func linkedinOrgURN(acc *trendlymodels.SocialAccount) string {
	if acc.RawProfile != nil {
		if u, ok := acc.RawProfile["orgUrn"].(string); ok && u != "" {
			return u
		}
	}
	if acc.PlatformAccountID != "" {
		return "urn:li:organization:" + acc.PlatformAccountID
	}
	return ""
}

// linkedinObjectFromComment extracts the embedded object (activity/share/ugcPost)
// URN from a LinkedIn comment URN, e.g.
//
//	urn:li:comment:(urn:li:activity:12345,67890) → urn:li:activity:12345
func linkedinObjectFromComment(commentURN string) string {
	open := strings.Index(commentURN, "(")
	comma := strings.LastIndex(commentURN, ",")
	if open == -1 || comma == -1 || comma <= open+1 {
		return ""
	}
	return commentURN[open+1 : comma]
}

func errUnsupported(msg string) error {
	return &unsupportedError{msg: msg}
}

type unsupportedError struct{ msg string }

func (e *unsupportedError) Error() string { return e.msg }

// logMediaErr is a tiny helper to keep the per-account error logging uniform.
func logMediaErr(brandID, socialID, platform string, err error) {
	if err != nil {
		log.Printf("inbox media: %s fetch failed for %s/%s: %v", platform, brandID, socialID, err)
	}
}
