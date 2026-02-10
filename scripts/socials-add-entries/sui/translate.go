package sui

import (
	"sort"
	"time"

	"github.com/idivarts/backend-sls/internal/models/trendlyrdb"
	"github.com/idivarts/backend-sls/pkg/apify"
	"github.com/lib/pq"
)

// TranslateInstagram converts Apify scraper output and manual overrides into
// the database models used for persistence.
func TranslateInstagram(ig apify.InstagramInfluencer, req ScrapedSocial) (*trendlyrdb.Socials, []trendlyrdb.InstagramPost) {
	now := time.Now().UnixMicro()

	// --- Build Socials ---
	social := &trendlyrdb.Socials{
		Username:        ig.Username,
		Name:            ig.FullName,
		Bio:             ig.Biography,
		ProfilePic:      ig.ProfilePicUrl,
		ProfilePicHD:    ig.ProfilePicUrlHD,
		Category:        ig.BusinessCategoryName,
		SocialType:      "instagram",
		ProfileVerified: ig.Verified,
		FollowerCount:   int64(ig.FollowersCount),
		FollowingCount:  int64(ig.FollowsCount),
		ContentCount:    int64(ig.PostsCount),
		Links:           translateLinks(ig.ExternalUrls),
		Niches:          req.Manual.Niches,
		QualityScore:    req.Manual.QualityScore,
		AddedBy:         "system",
		CreationTime:    now,
		LastUpdateTime:  now,
		ExternalId:      ig.Id,
	}
	social.ID = social.GetID()
	socialID := social.ID

	// --- Build Posts ---
	posts := make([]trendlyrdb.InstagramPost, 0, len(ig.LatestPosts))
	for _, p := range ig.LatestPosts {
		posts = append(posts, translatePost(p, socialID))
	}

	// --- Compute analytics from posts (mirrors old calculateFunctionLater logic) ---
	computeAnalytics(social, posts)

	return social, posts
}

// translateLinks converts Apify external URL objects to the DB Links model.
func translateLinks(urls []apify.InstagramExternalUrls) []trendlyrdb.Links {
	if len(urls) == 0 {
		return nil
	}
	links := make([]trendlyrdb.Links, len(urls))
	for i, u := range urls {
		links[i] = trendlyrdb.Links{
			Title:    u.Title,
			URL:      u.Url,
			LinkType: u.LinkType,
		}
	}
	return links
}

// translatePost converts a single Apify post (including its embedded reel
// data and child posts) into the DB InstagramPost model.
func translatePost(p apify.InstagramPosts, socialID string) trendlyrdb.InstagramPost {
	post := trendlyrdb.InstagramPost{
		ID:                 p.Id,
		SocialID:           socialID,
		Type:               p.Type,
		ShortCode:          p.ShortCode,
		Caption:            p.Caption,
		URL:                p.Url,
		DisplayURL:         p.DisplayUrl,
		VideoURL:           p.VideoUrl,
		LikesCount:         int64(p.LikesCount),
		CommentsCount:      int64(p.CommentsCount),
		VideoViewCount:     int64(p.VideoViewCount),
		VideoPlayCount:     int64(p.VideoPlayCount),
		VideoDuration:      p.VideoDuration,
		Timestamp:          p.Timestamp,
		LocationName:       p.LocationName,
		LocationID:         p.LocationId,
		IsPinned:           p.IsPinned,
		Alt:                p.Alt,
		Images:             pq.StringArray(p.Images),
		IsCommentsDisabled: p.IsCommentsDisabled,
		AudioURL:           p.AudioUrl,
		MusicInfo:          translateMusicInfo(p.MusicInfo),
		Hashtags:           pq.StringArray(p.Hashtags),
		Mentions:           pq.StringArray(p.Mentions),
		TaggedUsers:        translateTaggedUsers(p.TaggedUsers),
		FirstComment:       p.FirstComment,
		LatestComments:     translateComments(p.LatestComments),
	}

	// Recursively translate carousel / sidecar child posts.
	if len(p.ChildPosts) > 0 {
		post.ChildPosts = make([]trendlyrdb.InstagramPost, len(p.ChildPosts))
		for i, child := range p.ChildPosts {
			post.ChildPosts[i] = translatePost(child, socialID)
		}
	}

	return post
}

// translateMusicInfo converts an Apify MusicInfo to the DB pointer type.
// Returns nil when all fields are empty (no music attached).
func translateMusicInfo(m apify.InstagramMusicInfo) *trendlyrdb.MusicInfo {
	if m.ArtistName == "" && m.SongName == "" && m.AudioId == "" {
		return nil
	}
	return &trendlyrdb.MusicInfo{
		ArtistName:        m.ArtistName,
		SongName:          m.SongName,
		UsesOriginalAudio: m.UsesOriginalAudio,
		AudioID:           m.AudioId,
	}
}

// translateTaggedUsers converts Apify tagged-user objects to the DB User model.
func translateTaggedUsers(users []apify.InstagramTaggedUsers) []trendlyrdb.User {
	if len(users) == 0 {
		return nil
	}
	out := make([]trendlyrdb.User, len(users))
	for i, u := range users {
		out[i] = trendlyrdb.User{
			FullName:      u.FullName,
			ID:            u.Id,
			IsVerified:    u.IsVerified,
			ProfilePicURL: u.ProfilePicUrl,
			Username:      u.Username,
		}
	}
	return out
}

// translateComments converts Apify comment objects to the DB Comment model.
func translateComments(comments []apify.InstagramComment) []trendlyrdb.Comment {
	if len(comments) == 0 {
		return nil
	}
	out := make([]trendlyrdb.Comment, len(comments))
	for i, c := range comments {
		out[i] = trendlyrdb.Comment{
			ID:                 c.Id,
			Text:               c.Text,
			OwnerUsername:      c.OwnerUsername,
			OwnerProfilePicURL: c.OwnerProfilePicUrl,
			Timestamp:          c.Timestamp,
			LikesCount:         c.LikesCount,
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Analytics helpers
// ---------------------------------------------------------------------------

// computeAnalytics populates the aggregate/calculated fields on the social
// profile by iterating over its posts. Pinned posts are excluded from
// ViewsCount and EngagementCount totals (same rule as the legacy approach).
func computeAnalytics(social *trendlyrdb.Socials, posts []trendlyrdb.InstagramPost) {
	var (
		viewsList    []int64
		likesList    []int64
		commentsList []int64
		eRates       []float32
	)

	for _, p := range posts {
		views := p.VideoViewCount
		likes := p.LikesCount
		comments := p.CommentsCount

		if views > 0 {
			viewsList = append(viewsList, views)
		}
		if likes > 0 {
			likesList = append(likesList, likes)
		}
		if comments > 0 {
			commentsList = append(commentsList, comments)
		}

		// Totals exclude pinned posts.
		if !p.IsPinned {
			social.ViewsCount += views
			social.EngagementCount += likes + comments
		}

		// Per-post engagement rate for median calculation.
		if views > 0 {
			eRates = append(eRates, float32(likes+comments)*100/float32(views))
		}
	}

	social.AverageViews = medianInt64(viewsList)
	social.AverageLikes = medianInt64(likesList)
	social.AverageComments = medianInt64(commentsList)
	social.EngagementRate = medianFloat32(eRates)
}

// medianInt64 returns the median of an int64 slice as a float32.
func medianInt64(xs []int64) float32 {
	if len(xs) == 0 {
		return 0
	}
	sort.Slice(xs, func(i, j int) bool { return xs[i] < xs[j] })
	n := len(xs)
	if n%2 == 1 {
		return float32(xs[n/2])
	}
	return float32(xs[n/2-1]+xs[n/2]) / 2
}

// medianFloat32 returns the median of a float32 slice.
func medianFloat32(xs []float32) float32 {
	if len(xs) == 0 {
		return 0
	}
	sort.Slice(xs, func(i, j int) bool { return xs[i] < xs[j] })
	n := len(xs)
	if n%2 == 1 {
		return xs[n/2]
	}
	return (xs[n/2-1] + xs[n/2]) / 2
}
