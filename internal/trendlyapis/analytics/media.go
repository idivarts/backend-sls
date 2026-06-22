package analytics

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/instagram"
	"github.com/idivarts/backend-sls/pkg/messenger"
)

// Per-post (single media) basic analytics. Reuses the same Meta insight + media
// clients as the brand dashboard — the Content details page passes the published
// media id (from content.publishedIds) + its serving account so we never bulk
// store per-post metrics; they are read live from the Graph API on demand.

// PostMetric is a single scalar stat for a post (e.g. likes, reach).
type PostMetric struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Value     int64  `json:"value"`
	Available bool   `json:"available"`
}

// PostAnalytics is the basic-analytics payload for one published post.
type PostAnalytics struct {
	MediaID      string       `json:"mediaId"`
	Channel      string       `json:"channel"` // "instagram" | "facebook"
	MediaType    string       `json:"mediaType,omitempty"`
	Caption      string       `json:"caption,omitempty"`
	Permalink    string       `json:"permalink,omitempty"`
	ThumbnailURL string       `json:"thumbnailUrl,omitempty"`
	Timestamp    int64        `json:"timestamp,omitempty"` // Unix seconds
	Metrics      []PostMetric `json:"metrics"`
	FetchedAt    int64        `json:"fetchedAt"`
	Error        string       `json:"error,omitempty"`
}

// GetPostAnalytics returns basic analytics for a single published post.
// GET /api/v2/brands/:brandId/analytics/media/:mediaId?socialId=&channel=
func GetPostAnalytics(c *gin.Context) {
	brandID := c.Param("brandId")
	mediaID := c.Param("mediaId")
	socialID := c.Query("socialId")
	if brandID == "" || mediaID == "" || socialID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "brandId, mediaId and socialId are required"})
		return
	}
	channel := c.Query("channel")
	switch channel {
	case "":
		channel = trendlymodels.PlatformInstagram // default to Instagram
	case trendlymodels.PlatformInstagram, trendlymodels.PlatformFacebook:
		// ok
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported channel"})
		return
	}

	acc, err := trendlymodels.GetBrandSocialAccount(brandID, socialID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "connected account not found"})
		return
	}
	tok, err := trendlymodels.GetBrandSocialToken(brandID, socialID)
	if err != nil || tok == nil || tok.AccessToken == "" {
		c.JSON(http.StatusBadGateway, gin.H{"error": "connected account has no usable token"})
		return
	}

	if channel == trendlymodels.PlatformFacebook {
		c.JSON(http.StatusOK, fetchFacebookPost(mediaID, tok.AccessToken))
		return
	}
	c.JSON(http.StatusOK, fetchInstagramPost(*acc, tok.AccessToken, mediaID))
}

// igGraphType mirrors inbox.graphTypeForIG: directly-connected IG accounts read
// from the Instagram Graph (1); page-linked IG Business accounts via the FB Graph (0).
func igGraphType(acc trendlymodels.SocialAccount) int {
	if acc.Platform == trendlymodels.PlatformInstagram {
		return 1
	}
	return 0
}

func isVideoType(t string) bool {
	switch strings.ToUpper(t) {
	case "VIDEO", "REEL", "REELS":
		return true
	}
	return false
}

// fetchInstagramPost composes the media object (reliable like/comment counts)
// with media insights (reach/views/saves/shares/engagement, best-effort).
func fetchInstagramPost(acc trendlymodels.SocialAccount, at, mediaID string) PostAnalytics {
	out := PostAnalytics{MediaID: mediaID, Channel: trendlymodels.PlatformInstagram, FetchedAt: time.Now().Unix()}
	gt := igGraphType(acc)

	media, err := instagram.GetMediaByID(mediaID, at, gt)
	if err != nil {
		out.Error = "instagram media: " + err.Error()
	}
	var likes, comments int64
	if media != nil {
		out.MediaType = media.MediaType
		out.Caption = media.Caption
		out.Permalink = media.Permalink
		out.ThumbnailURL = firstNonEmpty(media.ThumbnailURL, media.MediaURL)
		out.Timestamp = unixOrZero(media.Timestamp)
		likes = int64(media.LikeCount)
		comments = int64(media.CommentsCount)
	}

	// Media-level insights (require the insights scope + a business/creator
	// account). A single unsupported metric fails the whole call, so we fall
	// back to reach-only on error.
	metrics := []instagram.InsightMetric{instagram.MetricReach, instagram.MetricTotalInteractions, "saved", instagram.MetricShares}
	if isVideoType(out.MediaType) {
		metrics = append(metrics, instagram.MetricViews)
	}
	ins, ierr := instagram.GetMediaInsights(mediaID, at, gt, metrics)
	if ierr != nil {
		ins, _ = instagram.GetMediaInsights(mediaID, at, gt, []instagram.InsightMetric{instagram.MetricReach})
	}

	has := func(m instagram.InsightMetric) bool { return ins != nil && ins.Find(m) != nil }
	val := func(m instagram.InsightMetric) int64 {
		if ins == nil {
			return 0
		}
		return ins.Total(m)
	}

	out.Metrics = []PostMetric{
		{Key: "likes", Label: "Likes", Value: likes, Available: media != nil},
		{Key: "comments", Label: "Comments", Value: comments, Available: media != nil},
		{Key: BucketReach, Label: "Reach", Value: val(instagram.MetricReach), Available: has(instagram.MetricReach)},
	}
	if isVideoType(out.MediaType) {
		out.Metrics = append(out.Metrics, PostMetric{Key: BucketViews, Label: "Views", Value: val(instagram.MetricViews), Available: has(instagram.MetricViews)})
	}
	out.Metrics = append(out.Metrics,
		PostMetric{Key: "saves", Label: "Saves", Value: val("saved"), Available: has("saved")},
		PostMetric{Key: "shares", Label: "Shares", Value: val(instagram.MetricShares), Available: has(instagram.MetricShares)},
		PostMetric{Key: BucketEngagement, Label: "Engagement", Value: val(instagram.MetricTotalInteractions), Available: has(instagram.MetricTotalInteractions)},
	)
	return out
}

// fetchFacebookPost composes the post object (likes/comments/shares) with a
// best-effort post-reach insight.
func fetchFacebookPost(postID, at string) PostAnalytics {
	out := PostAnalytics{MediaID: postID, Channel: trendlymodels.PlatformFacebook, FetchedAt: time.Now().Unix()}

	post, err := messenger.GetPostByID(postID, at)
	if err != nil {
		out.Error = "facebook post: " + err.Error()
	}
	var likes, comments, shares int64
	hasPost := post != nil
	if hasPost {
		out.Caption = post.Message
		out.Permalink = post.PermalinkURL
		out.ThumbnailURL = post.FullPicture
		out.MediaType = "POST"
		out.Timestamp = unixOrZero(post.CreatedTime)
		likes = int64(post.LikeCount())
		comments = int64(post.CommentCount())
		shares = int64(post.ShareCount())
	}

	// Post reach — best-effort; reuse the page-insights client against the post id.
	// NOTE: post_impressions_unique was deprecated by Meta on 2025-11-15 across all
	// API versions; post_total_media_view_unique is its unique-reach replacement
	// (per the v25.0 changelog).
	var reach int64
	reachAvailable := false
	if ins, rerr := messenger.GetFacebookInsights(postID, at,
		[]messenger.FBInsightMetric{messenger.FBMetricPostTotalMediaViewUnique},
		messenger.FBPeriodLifetime,
		messenger.FBInsightParams{},
	); rerr == nil && ins != nil && ins.Find(messenger.FBMetricPostTotalMediaViewUnique) != nil {
		reach = ins.Total(messenger.FBMetricPostTotalMediaViewUnique)
		reachAvailable = true
	} else if rerr != nil {
		log.Printf("analytics post: FB reach fetch failed for %s: %v", postID, rerr)
	}

	out.Metrics = []PostMetric{
		{Key: "likes", Label: "Likes", Value: likes, Available: hasPost},
		{Key: "comments", Label: "Comments", Value: comments, Available: hasPost},
		{Key: "shares", Label: "Shares", Value: shares, Available: hasPost},
		{Key: BucketReach, Label: "Reach", Value: reach, Available: reachAvailable},
		{Key: BucketEngagement, Label: "Engagement", Value: likes + comments + shares, Available: hasPost},
	}
	return out
}

// firstNonEmpty returns the first non-empty string.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
