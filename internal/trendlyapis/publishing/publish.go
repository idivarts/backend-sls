package publishing

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/idivarts/backend-sls/internal/constants"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/facebook"
	"github.com/idivarts/backend-sls/pkg/instagram"
	"github.com/idivarts/backend-sls/pkg/linkedin"
	"github.com/idivarts/backend-sls/pkg/reddit"
	"github.com/idivarts/backend-sls/pkg/twitter"
	"github.com/idivarts/backend-sls/pkg/youtube"
)

// platformTargeted reports whether p is among the content's targeted platforms.
func platformTargeted(p trendlymodels.Platform, list []trendlymodels.Platform) bool {
	for _, x := range list {
		if x == p {
			return true
		}
	}
	return false
}

// buildCaption merges caption + hashtags into the post body.
func buildCaption(ct *trendlymodels.Content) string {
	parts := []string{}
	if strings.TrimSpace(ct.Caption) != "" {
		parts = append(parts, strings.TrimSpace(ct.Caption))
	}
	if strings.TrimSpace(ct.Hashtags) != "" {
		parts = append(parts, strings.TrimSpace(ct.Hashtags))
	}
	return strings.Join(parts, "\n\n")
}

// isSentenceEnd reports whether r terminates a sentence/statement.
func isSentenceEnd(r rune) bool {
	return r == '.' || r == '!' || r == '?' || r == '…'
}

// splitTweetThread splits body into ≤limit-rune segments, never breaking
// mid-word and preferring a sentence boundary, then the last whitespace. Mirrors
// the frontend utils/twitter-thread.ts splitter.
func splitTweetThread(body string, limit int) []string {
	text := strings.TrimSpace(body)
	if text == "" {
		return []string{}
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return []string{text}
	}

	out := []string{}
	rest := runes
	for len(rest) > limit {
		window := rest[:limit]

		// Last sentence break in the window (index just after the terminator).
		sentenceAt := -1
		for i := 0; i < len(window); i++ {
			if isSentenceEnd(window[i]) && (i+1 >= len(window) || window[i+1] == ' ' || window[i+1] == '\n') {
				sentenceAt = i + 1
			}
		}
		// Last whitespace in the window.
		spaceAt := -1
		for i := len(window) - 1; i >= 0; i-- {
			if window[i] == ' ' || window[i] == '\n' || window[i] == '\t' {
				spaceAt = i
				break
			}
		}

		cut := 0
		switch {
		case sentenceAt > limit*2/5:
			cut = sentenceAt
		case spaceAt > 0:
			cut = spaceAt
		default:
			cut = limit // one giant token — hard split
		}

		piece := strings.TrimSpace(string(rest[:cut]))
		if piece != "" {
			out = append(out, piece)
		}
		rest = []rune(strings.TrimSpace(string(rest[cut:])))
	}
	if len(rest) > 0 {
		out = append(out, strings.TrimSpace(string(rest)))
	}
	return out
}

func firstImageURL(ct *trendlymodels.Content) string {
	for _, a := range ct.Attachments {
		if a.ImageURL != "" {
			return a.ImageURL
		}
	}
	return ""
}

func firstVideoURL(ct *trendlymodels.Content) string {
	for _, a := range ct.Attachments {
		if a.PlayURL != "" {
			return a.PlayURL
		}
		if a.AppleURL != "" {
			return a.AppleURL
		}
	}
	return ""
}

func imageURLs(ct *trendlymodels.Content) []string {
	urls := []string{}
	for _, a := range ct.Attachments {
		if a.ImageURL != "" {
			urls = append(urls, a.ImageURL)
		}
	}
	return urls
}

// waitForContainer polls a video container until processing finishes.
func waitForContainer(containerID, accessToken string) error {
	for i := 0; i < 20; i++ {
		status, err := instagram.GetContainerStatus(containerID, accessToken)
		if err != nil {
			return err
		}
		switch status {
		case "FINISHED":
			return nil
		case "ERROR", "EXPIRED":
			return fmt.Errorf("media container processing failed: %s", status)
		}
		time.Sleep(3 * time.Second)
	}
	return fmt.Errorf("media container did not finish processing in time")
}

// publishToInstagram runs the two-step IG publish appropriate to the format.
func publishToInstagram(igUserID, accessToken string, ct *trendlymodels.Content) (string, error) {
	caption := buildCaption(ct)
	format := strings.ToLower(ct.ContentFormat)

	var creationID string
	var err error

	switch format {
	case "reel", "video":
		// Instagram publishes all feed video (portrait Reel or landscape video)
		// through the REELS container — the aspect ratio lives in the asset.
		video := firstVideoURL(ct)
		if video == "" {
			return "", fmt.Errorf("video content has no video attachment")
		}
		creationID, err = instagram.CreateReelContainer(igUserID, video, caption, "REELS", accessToken)
		if err != nil {
			return "", err
		}
	case "story":
		img := firstImageURL(ct)
		if img == "" {
			return "", fmt.Errorf("story has no image attachment")
		}
		creationID, err = instagram.CreateStoryImageContainer(igUserID, img, accessToken)
		if err != nil {
			return "", err
		}
	case "carousel":
		urls := imageURLs(ct)
		if len(urls) < 2 {
			return "", fmt.Errorf("carousel needs at least 2 images")
		}
		childIDs := []string{}
		for _, u := range urls {
			cid, cerr := instagram.CreateCarouselItem(igUserID, u, accessToken)
			if cerr != nil {
				return "", cerr
			}
			if werr := waitForContainer(cid, accessToken); werr != nil {
				return "", werr
			}
			childIDs = append(childIDs, cid)
		}
		creationID, err = instagram.CreateCarouselContainer(igUserID, childIDs, caption, accessToken)
		if err != nil {
			return "", err
		}
	case "text":
		// Instagram has no plain-text post format; these target FB / LinkedIn / X.
		return "", fmt.Errorf("instagram does not support text-only posts")
	default: // post
		img := firstImageURL(ct)
		if img == "" {
			return "", fmt.Errorf("post has no image attachment")
		}
		creationID, err = instagram.CreateImageContainer(igUserID, img, caption, accessToken)
		if err != nil {
			return "", err
		}
	}

	if err = waitForContainer(creationID, accessToken); err != nil {
		return "", err
	}
	return instagram.PublishContainer(igUserID, creationID, accessToken)
}

// publishToFacebook posts to a Facebook Page (photo if an image exists, else text).
func publishToFacebook(pageID, pageToken string, ct *trendlymodels.Content) (string, error) {
	caption := buildCaption(ct)
	img := firstImageURL(ct)

	var res *facebook.FBPublishResponse
	var err error
	if img != "" {
		res, err = facebook.PublishPagePhoto(pageID, img, caption, pageToken)
	} else {
		res, err = facebook.PublishPageFeed(pageID, caption, "", pageToken)
	}
	if err != nil {
		return "", err
	}
	if res.PostID != "" {
		return res.PostID, nil
	}
	return res.ID, nil
}

// publishToLinkedIn posts to a member's personal LinkedIn profile. The member
// URN was stored in the account's raw profile (`sub`) at connect time.
func publishToLinkedIn(account *trendlymodels.SocialAccount, accessToken string, ct *trendlymodels.Content) (string, error) {
	sub, _ := account.RawProfile["sub"].(string)
	if sub == "" {
		return "", fmt.Errorf("linkedin account %s has no member id", account.ID)
	}
	// LinkedIn's OIDC /userinfo returns `sub` as a bare member id (e.g.
	// "Au3Lx1cikz"), but the Posts API requires a full member URN. Wrap it
	// unless it's already a urn:... value.
	authorURN := sub
	if !strings.HasPrefix(authorURN, "urn:") {
		authorURN = "urn:li:person:" + sub
	}
	return linkedin.CreateMemberPost(accessToken, authorURN, buildCaption(ct), imageURLs(ct))
}

// orgURNForAccount derives the organization URN for a linkedin_page account from
// its stored orgUrn (preferred) or its PlatformAccountID (the numeric org id).
func orgURNForAccount(account *trendlymodels.SocialAccount) string {
	if account.RawProfile != nil {
		if u, ok := account.RawProfile["orgUrn"].(string); ok && u != "" {
			return u
		}
	}
	if account.PlatformAccountID != "" {
		return "urn:li:organization:" + account.PlatformAccountID
	}
	return ""
}

// publishToLinkedInPage posts to a LinkedIn Company/Showcase Page (org feed) via
// the Community Management API.
func publishToLinkedInPage(account *trendlymodels.SocialAccount, accessToken string, ct *trendlymodels.Content) (string, error) {
	orgURN := orgURNForAccount(account)
	if orgURN == "" {
		return "", fmt.Errorf("linkedin page account %s has no organization urn", account.ID)
	}
	return linkedin.CreateOrgPost(accessToken, orgURN, buildCaption(ct), imageURLs(ct))
}

// tweetSegments returns the ordered tweets to post for this content: the
// variation's explicit thread when set, otherwise the caption auto-split into
// ≤280-char tweets (never breaking mid-word). A single-element result is a
// normal one-off tweet.
func tweetSegments(ct *trendlymodels.Content) []string {
	if ct.PlatformOptions != nil && len(ct.PlatformOptions.TwitterThread) > 0 {
		out := []string{}
		for _, t := range ct.PlatformOptions.TwitterThread {
			if s := strings.TrimSpace(t); s != "" {
				out = append(out, s)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return splitTweetThread(buildCaption(ct), 280)
}

// publishToTwitter posts a tweet, or a self-reply thread when the content is too
// long / an explicit thread was authored. Media rides on the first tweet. The
// shared scheduler handles timing, so this always publishes immediately.
func publishToTwitter(accessToken string, ct *trendlymodels.Content) (string, error) {
	segments := tweetSegments(ct)
	if len(segments) == 0 {
		return "", fmt.Errorf("twitter: post has no text")
	}
	// First tweet carries any media (up to 4 images, or one video).
	firstID, err := twitter.PublishTweet(accessToken, segments[0], imageURLs(ct), firstVideoURL(ct))
	if err != nil {
		return "", err
	}
	replyTo := firstID
	for _, seg := range segments[1:] {
		id, rerr := twitter.ReplyToTweet(accessToken, replyTo, seg)
		if rerr != nil {
			// The thread is partially posted — surface the failure but keep the
			// first tweet's id so the post is still recorded.
			return firstID, fmt.Errorf("twitter: thread reply failed after %d tweet(s): %w", len(segments)-len(segments[1:]), rerr)
		}
		replyTo = id
	}
	return firstID, nil
}

// publishToYouTube uploads a video (or Short) to the connected channel. A video
// attachment is required; the title comes from platform options or the content
// title, the description from the caption. We publish immediately (privacy from
// options, default public) — the shared scheduler owns timing, so we do NOT use
// YouTube's native publishAt.
func publishToYouTube(accessToken string, ct *trendlymodels.Content) (string, error) {
	video := firstVideoURL(ct)
	if video == "" {
		return "", fmt.Errorf("youtube requires a video attachment")
	}
	title := strings.TrimSpace(ct.Title)
	privacy := "public"
	madeForKids := false
	if ct.PlatformOptions != nil {
		if t := strings.TrimSpace(ct.PlatformOptions.YouTubeTitle); t != "" {
			title = t
		}
		if p := strings.TrimSpace(ct.PlatformOptions.YouTubePrivacy); p != "" {
			privacy = p
		}
		madeForKids = ct.PlatformOptions.YouTubeMadeForKids
	}
	if title == "" {
		title = "Untitled"
	}
	desc := buildCaption(ct)
	// A variation may set a dedicated YouTube description distinct from the caption.
	if ct.PlatformOptions != nil {
		if d := strings.TrimSpace(ct.PlatformOptions.YouTubeDescription); d != "" {
			desc = d
		}
	}
	// A "reel" maps to a YouTube Short — tag #Shorts so YouTube classifies it
	// (there is no dedicated Shorts upload endpoint).
	if strings.EqualFold(ct.ContentFormat, "reel") && !strings.Contains(strings.ToLower(desc), "#shorts") {
		desc = strings.TrimSpace(desc + "\n#Shorts")
	}
	return youtube.PublishVideo(accessToken, youtube.UploadOptions{
		Title:         title,
		Description:   desc,
		PrivacyStatus: privacy,
		VideoURL:      video,
		MadeForKids:   madeForKids,
	})
}

// publishToReddit submits a post to the chosen subreddit. Subreddit + title are
// required (collected via platform options). An image attachment → image post,
// otherwise a self (text) post. Returns the post fullname (t3_…).
func publishToReddit(accessToken string, ct *trendlymodels.Content) (string, error) {
	opt := reddit.SubmitOptions{}
	if ct.PlatformOptions != nil {
		opt.Subreddit = strings.TrimSpace(ct.PlatformOptions.RedditSubreddit)
		opt.Title = strings.TrimSpace(ct.PlatformOptions.RedditTitle)
		opt.FlairID = ct.PlatformOptions.RedditFlairID
		opt.NSFW = ct.PlatformOptions.RedditNSFW
	}
	if opt.Subreddit == "" {
		return "", fmt.Errorf("reddit requires a target subreddit")
	}
	if opt.Title == "" {
		opt.Title = strings.TrimSpace(ct.Title)
	}
	if opt.Title == "" {
		return "", fmt.Errorf("reddit requires a post title")
	}
	body := buildCaption(ct)
	if img := firstImageURL(ct); img != "" {
		opt.Kind = "image"
		opt.ImageURLs = []string{img}
		opt.Text = body
	} else {
		opt.Kind = "self"
		opt.Text = body
	}
	fullname, _, err := reddit.Submit(accessToken, opt)
	if err != nil {
		return "", err
	}
	return fullname, nil
}

// PublishContent loads a content doc and publishes it to each destination,
// recording per-platform published ids and a final status on the document.
func PublishContent(brandID, contentID string) error {
	ct, err := trendlymodels.GetContent(brandID, contentID)
	if err != nil {
		return err
	}
	if len(ct.Destinations) == 0 {
		return fmt.Errorf("content %s has no destinations", contentID)
	}

	// Per-platform variations override the generic content at publish time. A
	// missing variation → that platform publishes the generic content unchanged.
	variations, verr := trendlymodels.ListContentVariations(brandID, contentID)
	if verr != nil {
		log.Printf("publishing: could not load variations for content %s (using generic): %v", contentID, verr)
		variations = map[string]*trendlymodels.ContentVariation{}
	}

	publishedIds := map[string]string{}
	var firstErr error

	for _, dest := range ct.Destinations {
		// Never publish to a platform the content isn't targeting. (Legacy docs
		// with no `platforms` set skip this guard.)
		if len(ct.Platforms) > 0 && !platformTargeted(dest.Platform, ct.Platforms) {
			log.Printf("publishing: destination platform %q not in content %s targeted platforms; skipping", dest.Platform, contentID)
			continue
		}
		// Effective content for THIS platform = generic ⊕ its variation override.
		eff := ct.EffectiveForPlatform(variations[string(dest.Platform)])
		account, aerr := trendlymodels.GetBrandSocialAccount(brandID, dest.SocialAccountID)
		if aerr != nil {
			if firstErr == nil {
				firstErr = aerr
			}
			continue
		}
		// Resolve via the account so linkedin_page Pages (which share one member
		// token doc via TokenRef) read the right token; all other platforms have
		// an empty TokenRef and behave identically to a by-id lookup.
		token, terr := trendlymodels.GetBrandSocialTokenForAccount(brandID, account)
		if terr != nil {
			if firstErr == nil {
				firstErr = terr
			}
			continue
		}

		switch dest.Platform {
		case "instagram":
			id, perr := publishToInstagram(account.PlatformAccountID, token.AccessToken, eff)
			if perr != nil {
				if firstErr == nil {
					firstErr = perr
				}
				continue
			}
			publishedIds["instagram"] = id
		case "facebook":
			id, perr := publishToFacebook(account.PlatformAccountID, token.AccessToken, eff)
			if perr != nil {
				if firstErr == nil {
					firstErr = perr
				}
				continue
			}
			publishedIds["facebook"] = id
		case "linkedin":
			id, perr := publishToLinkedIn(account, token.AccessToken, eff)
			if perr != nil {
				if firstErr == nil {
					firstErr = perr
				}
				continue
			}
			publishedIds["linkedin"] = id
		case "linkedin_page":
			id, perr := publishToLinkedInPage(account, token.AccessToken, eff)
			if perr != nil {
				if firstErr == nil {
					firstErr = perr
				}
				continue
			}
			publishedIds["linkedin_page"] = id
		case "twitter":
			id, perr := publishToTwitter(token.AccessToken, eff)
			if perr != nil {
				if firstErr == nil {
					firstErr = perr
				}
				continue
			}
			publishedIds["twitter"] = id
		case "youtube":
			id, perr := publishToYouTube(token.AccessToken, eff)
			if perr != nil {
				if firstErr == nil {
					firstErr = perr
				}
				continue
			}
			publishedIds["youtube"] = id
		case "reddit":
			if !constants.RedditEnabled {
				if firstErr == nil {
					firstErr = fmt.Errorf("reddit integration is not enabled")
				}
				continue
			}
			id, perr := publishToReddit(token.AccessToken, eff)
			if perr != nil {
				if firstErr == nil {
					firstErr = perr
				}
				continue
			}
			publishedIds["reddit"] = id
		default:
			log.Printf("publishing: unsupported platform %q for content %s", dest.Platform, contentID)
		}
	}

	fields := map[string]interface{}{
		"publishedIds": publishedIds,
	}
	if len(publishedIds) > 0 {
		// The post is live now (whether via publish-now or a scheduled job that
		// just fired). Stamp the actual posting time onto both the precise
		// publish field and the calendar-placement field so the calendar shows
		// the post when it really went out, not at a stale scheduled time.
		postedAt := time.Now().UnixMilli()
		fields["status"] = "posted"
		fields["scheduledAt"] = postedAt
		fields["postingTimeStamp"] = postedAt
	}
	if firstErr != nil {
		fields["publishError"] = firstErr.Error()
	} else {
		fields["publishError"] = ""
	}
	if uerr := trendlymodels.UpdateContentFields(brandID, contentID, fields); uerr != nil {
		log.Printf("publishing: failed to update content %s: %v", contentID, uerr)
	}

	return firstErr
}
