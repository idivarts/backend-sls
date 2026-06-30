package trendlymodels

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

// Content mirrors the brand-app content document at
// brands/{brandId}/contents/{contentId}. Only the fields the backend
// publishing pipeline needs are modelled here.

type ContentAttachment struct {
	Type     string `json:"type" firestore:"type"`
	ImageURL string `json:"imageUrl,omitempty" firestore:"imageUrl"`
	PlayURL  string `json:"playUrl,omitempty" firestore:"playUrl"`
	AppleURL string `json:"appleUrl,omitempty" firestore:"appleUrl"`
}

type ContentDestination struct {
	SocialAccountID string `json:"socialAccountId" firestore:"socialAccountId"`
	Platform        string `json:"platform" firestore:"platform"`
	Username        string `json:"username,omitempty" firestore:"username"`
}

// ContentPlatformOptions holds per-platform publishing extras that don't fit the
// shared caption/attachment model. Optional; only the fields for a content's
// targeted platforms are read at publish time. Mirrors the frontend
// IPlatformOptions (a flat, prefix-namespaced bag) — keep the two in sync.
type ContentPlatformOptions struct {
	// Instagram
	InstagramLocation     string `json:"instagramLocation,omitempty" firestore:"instagramLocation,omitempty"`
	InstagramAltText      string `json:"instagramAltText,omitempty" firestore:"instagramAltText,omitempty"`
	InstagramFirstComment string `json:"instagramFirstComment,omitempty" firestore:"instagramFirstComment,omitempty"`
	// Facebook
	FacebookFirstComment string `json:"facebookFirstComment,omitempty" firestore:"facebookFirstComment,omitempty"`
	// LinkedIn (personal + page)
	LinkedInVisibility   string `json:"linkedinVisibility,omitempty" firestore:"linkedinVisibility,omitempty"` // PUBLIC|CONNECTIONS|LOGGED_IN
	LinkedInFirstComment string `json:"linkedinFirstComment,omitempty" firestore:"linkedinFirstComment,omitempty"`
	LinkedInAltText      string `json:"linkedinAltText,omitempty" firestore:"linkedinAltText,omitempty"`
	// Twitter / X — a thread of >1 entry publishes as a self-reply chain.
	TwitterThread        []string `json:"twitterThread,omitempty" firestore:"twitterThread,omitempty"`
	TwitterReplySettings string   `json:"twitterReplySettings,omitempty" firestore:"twitterReplySettings,omitempty"`
	TwitterQuoteTweetID  string   `json:"twitterQuoteTweetId,omitempty" firestore:"twitterQuoteTweetId,omitempty"`
	TwitterAltText       string   `json:"twitterAltText,omitempty" firestore:"twitterAltText,omitempty"`
	// YouTube — a video needs a title + visibility distinct from the caption.
	YouTubeTitle       string   `json:"youtubeTitle,omitempty" firestore:"youtubeTitle,omitempty"`
	YouTubeDescription string   `json:"youtubeDescription,omitempty" firestore:"youtubeDescription,omitempty"`
	YouTubeTags        []string `json:"youtubeTags,omitempty" firestore:"youtubeTags,omitempty"`
	YouTubeCategoryID  string   `json:"youtubeCategoryId,omitempty" firestore:"youtubeCategoryId,omitempty"`
	YouTubePrivacy     string   `json:"youtubePrivacy,omitempty" firestore:"youtubePrivacy,omitempty"` // public|private|unlisted
	YouTubeMadeForKids bool     `json:"youtubeMadeForKids,omitempty" firestore:"youtubeMadeForKids,omitempty"`
	YouTubePlaylistID  string   `json:"youtubePlaylistId,omitempty" firestore:"youtubePlaylistId,omitempty"`
	// Reddit — a submission needs a target subreddit + title (+ optional flair).
	RedditSubreddit   string `json:"redditSubreddit,omitempty" firestore:"redditSubreddit,omitempty"`
	RedditTitle       string `json:"redditTitle,omitempty" firestore:"redditTitle,omitempty"`
	RedditFlairID     string `json:"redditFlairId,omitempty" firestore:"redditFlairId,omitempty"`
	RedditFlairText   string `json:"redditFlairText,omitempty" firestore:"redditFlairText,omitempty"`
	RedditNSFW        bool   `json:"redditNsfw,omitempty" firestore:"redditNsfw,omitempty"`
	RedditSpoiler     bool   `json:"redditSpoiler,omitempty" firestore:"redditSpoiler,omitempty"`
	RedditSendReplies bool   `json:"redditSendReplies,omitempty" firestore:"redditSendReplies,omitempty"`
}

// ContentImageGeneration tracks the live state of an AI image-generation job on
// the content doc. It is written by the websocket image handler so the brand app
// can render progress and the finished image from its Firestore subscription —
// independent of the websocket connection that kicked the job off.
type ContentImageGeneration struct {
	Status         string `json:"status" firestore:"status"` // "generating" | "done" | "error"
	Prompt         string `json:"prompt,omitempty" firestore:"prompt"`
	Error          string `json:"error,omitempty" firestore:"error"`
	RequestedCount int    `json:"requestedCount,omitempty" firestore:"requestedCount"`
	CompletedCount int    `json:"completedCount,omitempty" firestore:"completedCount"`
	StartedAt      int64  `json:"startedAt,omitempty" firestore:"startedAt"`
	UpdatedAt      int64  `json:"updatedAt,omitempty" firestore:"updatedAt"`
}

type Content struct {
	ID            string        `json:"id,omitempty" firestore:"-"`
	Title         string        `json:"title" firestore:"title"`
	Caption       string        `json:"caption,omitempty" firestore:"caption"`
	Hashtags      string        `json:"hashtags,omitempty" firestore:"hashtags"`
	Script        string        `json:"script,omitempty" firestore:"script"`
	Description   string        `json:"description,omitempty" firestore:"description"`
	Status        string        `json:"status" firestore:"status"`
	ContentFormat ContentFormat `json:"contentFormat" firestore:"contentFormat"`
	// Platforms this content is planned for (the publishing INTENT). Each
	// Destination below must target one of these platforms.
	Platforms []Platform `json:"platforms,omitempty" firestore:"platforms"`
	// Platform is the deprecated legacy single-platform field (capitalised
	// string, e.g. "Instagram"). Superseded by Platforms; read for back-compat
	// coercion of old docs only — never written by new code.
	Platform         string                  `json:"platform,omitempty" firestore:"platform"`
	ManagerID        string                  `json:"managerId,omitempty" firestore:"managerId"`
	StrategyID       string                  `json:"strategyId,omitempty" firestore:"strategyId"`
	PostingTimeStamp int64                   `json:"postingTimeStamp,omitempty" firestore:"postingTimeStamp"`
	IsArchived       bool                    `json:"isArchived,omitempty" firestore:"isArchived"`
	Attachments      []ContentAttachment     `json:"attachments,omitempty" firestore:"attachments"`
	Destinations     []ContentDestination    `json:"destinations,omitempty" firestore:"destinations"`
	PlatformOptions  *ContentPlatformOptions `json:"platformOptions,omitempty" firestore:"platformOptions,omitempty"`
	ImageGeneration  *ContentImageGeneration `json:"imageGeneration,omitempty" firestore:"imageGeneration"`
	// MediaConversationID is the dedicated AI thread (ai_conversations doc,
	// module="media") for this content's image generate/enhance iterations.
	// Stamped on first generation, loaded directly on enhance (no index needed).
	MediaConversationID  string                 `json:"mediaConversationId,omitempty" firestore:"mediaConversationId,omitempty"`
	ScheduleMode         string                 `json:"scheduleMode,omitempty" firestore:"scheduleMode"`
	ScheduledAt          int64                  `json:"scheduledAt,omitempty" firestore:"scheduledAt"`
	ScheduleExecutionArn string                 `json:"scheduleExecutionArn,omitempty" firestore:"scheduleExecutionArn"`
	PublishedIds         map[string]string      `json:"publishedIds,omitempty" firestore:"publishedIds"`
	PublishError         string                 `json:"publishError,omitempty" firestore:"publishError"`
	PostedURL            string                 `json:"postedUrl,omitempty" firestore:"postedUrl"`
	Metrics              map[string]interface{} `json:"metrics,omitempty" firestore:"metrics"`
	CreatedAt            int64                  `json:"createdAt,omitempty" firestore:"createdAt"`
	UpdatedAt            int64                  `json:"updatedAt,omitempty" firestore:"updatedAt"`
}

func contentsCollection(brandID string) *firestore.CollectionRef {
	return firestoredb.Client.Collection(fmt.Sprintf("brands/%s/contents", brandID))
}

// normalize back-fills the new shape from legacy fields on read so old
// documents (which stored a single capitalised `platform` string) behave like
// new ones. Mutates the receiver in place.
func (ct *Content) normalize() {
	if len(ct.Platforms) == 0 && ct.Platform != "" {
		if p, ok := NormalizePlatform(ct.Platform); ok {
			ct.Platforms = []Platform{p}
		}
	}
	if ct.ContentFormat != "" {
		ct.ContentFormat = NormalizeContentFormat(ct.ContentFormat)
	}
}

// GetContent reads a single content document for a brand, populating ID.
func GetContent(brandID, contentID string) (*Content, error) {
	doc, err := contentsCollection(brandID).Doc(contentID).Get(context.Background())
	if err != nil {
		return nil, err
	}
	var ct Content
	if err := doc.DataTo(&ct); err != nil {
		return nil, err
	}
	ct.ID = doc.Ref.ID
	ct.normalize()
	return &ct, nil
}

// CreateContent adds a new content document and returns its generated id. The
// caller supplies the field map (calendar/onboarding/push-to-calendar all stamp
// their own seed fields); createdAt/updatedAt are filled in only when absent.
func CreateContent(ctx context.Context, brandID string, fields map[string]interface{}) (string, error) {
	now := time.Now().UnixMilli()
	if _, ok := fields["createdAt"]; !ok {
		fields["createdAt"] = now
	}
	if _, ok := fields["updatedAt"]; !ok {
		fields["updatedAt"] = now
	}
	ref, _, err := contentsCollection(brandID).Add(ctx, fields)
	if err != nil {
		return "", err
	}
	return ref.ID, nil
}

// UpdateContent applies a partial update to a content document, always bumping
// updatedAt. Callers build the []firestore.Update; the Firestore call lives here.
func UpdateContent(ctx context.Context, brandID, contentID string, updates []firestore.Update) error {
	updates = append(updates, firestore.Update{Path: "updatedAt", Value: time.Now().UnixMilli()})
	_, err := contentsCollection(brandID).Doc(contentID).Update(ctx, updates)
	return err
}

// UpdateContentFields merge-updates the given fields on a content document and
// always bumps updatedAt.
func UpdateContentFields(brandID, contentID string, fields map[string]interface{}) error {
	fields["updatedAt"] = time.Now().UnixMilli()
	_, err := contentsCollection(brandID).
		Doc(contentID).
		Set(context.Background(), fields, firestore.MergeAll)
	return err
}

// ListContentInRange returns a brand's content whose postingTimeStamp falls in
// [start, end), ordered ascending. When includeArchived is false, archived
// (soft-deleted) items are skipped. Each item carries its document ID.
func ListContentInRange(ctx context.Context, brandID string, start, end int64, includeArchived bool) ([]Content, error) {
	iter := contentsCollection(brandID).
		Where("postingTimeStamp", ">=", start).
		Where("postingTimeStamp", "<", end).
		OrderBy("postingTimeStamp", firestore.Asc).
		Documents(ctx)
	defer iter.Stop()

	out := []Content{}
	for {
		doc, err := iter.Next()
		if err != nil {
			break
		}
		var ct Content
		if err := doc.DataTo(&ct); err != nil {
			continue
		}
		if ct.IsArchived && !includeArchived {
			continue
		}
		ct.ID = doc.Ref.ID
		ct.normalize()
		out = append(out, ct)
	}
	return out, nil
}

// protectedContentStatuses are content lifecycle states that push-to-calendar's
// "replace existing window" path must NEVER delete. Anything the user has already
// committed to publishing — scheduled, approved-for-scheduling, or already
// posted — is preserved; only unscheduled drafts get cleared.
var protectedContentStatuses = map[string]bool{
	"scheduled": true,
	"approved":  true,
	"posted":    true,
}

// DeleteContentInRange deletes content documents whose postingTimeStamp falls in
// [start, end), skipping any item the user has already committed to publishing
// (scheduled / approved / posted, or anything with an active schedule execution
// or published platform ids). Returns the deleted document ids. Used by
// push-to-calendar's "replace existing window" path.
func DeleteContentInRange(ctx context.Context, brandID string, start, end int64) ([]string, error) {
	iter := contentsCollection(brandID).
		Where("postingTimeStamp", ">=", start).
		Where("postingTimeStamp", "<", end).
		Documents(ctx)
	defer iter.Stop()

	removed := []string{}
	for {
		doc, err := iter.Next()
		if err != nil {
			break
		}
		var ct Content
		if err := doc.DataTo(&ct); err != nil {
			// Couldn't decode — err on the safe side and leave it untouched.
			continue
		}
		// Never delete content that's scheduled or already posted.
		if protectedContentStatuses[ct.Status] || ct.ScheduleExecutionArn != "" || len(ct.PublishedIds) > 0 {
			continue
		}
		if _, e := doc.Ref.Delete(ctx); e == nil {
			removed = append(removed, doc.Ref.ID)
		}
	}
	return removed, nil
}

// ListContentByStatus returns a brand's content documents in the given status
// (e.g. "posted"). Each item carries its document ID.
func ListContentByStatus(ctx context.Context, brandID, status string) ([]Content, error) {
	iter := contentsCollection(brandID).
		Where("status", "==", status).
		Documents(ctx)
	defer iter.Stop()

	out := []Content{}
	for {
		doc, err := iter.Next()
		if err != nil {
			break
		}
		var ct Content
		if err := doc.DataTo(&ct); err != nil {
			continue
		}
		ct.ID = doc.Ref.ID
		ct.normalize()
		out = append(out, ct)
	}
	return out, nil
}
