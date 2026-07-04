package publishing

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/idivarts/backend-sls/internal/constants"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/internal/socialtokens"
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

// publishToFacebook posts to a Facebook Page: a video if the content has one
// (reel/video), else a photo if there's an image, else a plain text status.
func publishToFacebook(pageID, pageToken string, ct *trendlymodels.Content) (string, error) {
	caption := buildCaption(ct)
	img := firstImageURL(ct)
	video := firstVideoURL(ct)

	var res *facebook.FBPublishResponse
	var err error
	switch {
	case video != "":
		res, err = facebook.PublishPageVideo(pageID, video, caption, pageToken)
	case img != "":
		res, err = facebook.PublishPagePhoto(pageID, img, caption, pageToken)
	default:
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
	if video := firstVideoURL(ct); video != "" {
		return linkedin.CreateMemberVideoPost(accessToken, authorURN, buildCaption(ct), video)
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
	if video := firstVideoURL(ct); video != "" {
		return linkedin.CreateOrgVideoPost(accessToken, orgURN, buildCaption(ct), video)
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

// Per-destination publish states written to Content.PublishResults.
const (
	pubStatusPublishing = "publishing"
	pubStatusPublished  = "published"
	pubStatusFailed     = "failed"
	pubStatusSkipped    = "skipped"
)

// classifyPublishError maps a platform failure to a recovery hint the UI uses to
// pick the right action: "auth" → reconnect the account, "validation" → the user
// must fix the content (wrong media, missing field, too long), "transient" →
// safe to retry as-is. A coarse heuristic over the error text — good enough to
// steer the CTA; the human message still carries the detail.
func classifyPublishError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "token"), strings.Contains(msg, "unauthor"),
		strings.Contains(msg, "permission"), strings.Contains(msg, "oauth"),
		strings.Contains(msg, "expired"), strings.Contains(msg, "401"), strings.Contains(msg, "reconnect"):
		return "auth"
	case strings.Contains(msg, "no "), strings.Contains(msg, "requires"),
		strings.Contains(msg, "needs"), strings.Contains(msg, "must "),
		strings.Contains(msg, "at least"), strings.Contains(msg, "does not support"),
		strings.Contains(msg, "unsupported"), strings.Contains(msg, "too long"),
		strings.Contains(msg, "invalid"), strings.Contains(msg, "not enabled"):
		return "validation"
	default:
		return "transient"
	}
}

// publishDestination publishes the content (with its per-platform variation
// applied) to a single destination and returns the platform's post id.
func publishDestination(brandID string, ct *trendlymodels.Content, dest trendlymodels.ContentDestination, variation *trendlymodels.ContentVariation) (string, error) {
	// Effective content for THIS platform = generic ⊕ its variation override.
	eff := ct.EffectiveForPlatform(variation)
	account, aerr := trendlymodels.GetBrandSocialAccount(brandID, dest.SocialAccountID)
	if aerr != nil {
		return "", aerr
	}
	// Resolve via the account so linkedin_page Pages (which share one member
	// token doc via TokenRef) read the right token; all other platforms have an
	// empty TokenRef and behave identically to a by-id lookup.
	token, terr := trendlymodels.GetBrandSocialTokenForAccount(brandID, account)
	if terr != nil {
		return "", terr
	}
	// Short-lived tokens (YouTube ~1h, Twitter ~2h) expire long before the refresh
	// cron runs, so refresh just-in-time — otherwise an upload an hour after connect
	// 401s with "Invalid Credentials".
	token = socialtokens.EnsureFreshBrandToken(brandID, account, token)
	switch dest.Platform {
	case "instagram":
		return publishToInstagram(account.PlatformAccountID, token.AccessToken, eff)
	case "facebook":
		return publishToFacebook(account.PlatformAccountID, token.AccessToken, eff)
	case "linkedin":
		return publishToLinkedIn(account, token.AccessToken, eff)
	case "linkedin_page":
		return publishToLinkedInPage(account, token.AccessToken, eff)
	case "twitter":
		return publishToTwitter(token.AccessToken, eff)
	case "youtube":
		return publishToYouTube(token.AccessToken, eff)
	case "reddit":
		if !constants.RedditEnabled {
			return "", fmt.Errorf("reddit integration is not enabled")
		}
		return publishToReddit(token.AccessToken, eff)
	default:
		return "", fmt.Errorf("unsupported platform %q", dest.Platform)
	}
}

// deriveOverallStatus maps the per-destination results to the content-wide
// status the badge shows: any still in flight → "publishing"; a mix of
// success + failure → "partially_failed"; all-good → "posted"; all-bad →
// "failed". Skipped rows don't count either way.
func deriveOverallStatus(results []trendlymodels.ContentPublishResult) string {
	published, failed, publishing := 0, 0, 0
	for _, r := range results {
		switch r.Status {
		case pubStatusPublished:
			published++
		case pubStatusFailed:
			failed++
		case pubStatusPublishing:
			publishing++
		}
	}
	switch {
	case publishing > 0:
		return "publishing"
	case published > 0 && failed > 0:
		return "partially_failed"
	case published > 0:
		return "posted"
	case failed > 0:
		return "failed"
	default:
		return "publishing"
	}
}

// writePublishState persists the current per-destination results + derived
// overall status. When final, it also rebuilds the backward-compat publishedIds
// map + publishError and (if anything went live) stamps the real posting time.
func writePublishState(brandID, contentID string, results []trendlymodels.ContentPublishResult, final bool) {
	fields := map[string]interface{}{
		"publishResults": results,
		"status":         deriveOverallStatus(results),
	}
	if final {
		publishedIds := map[string]string{}
		firstErr := ""
		anyPublished := false
		for _, r := range results {
			if r.Status == pubStatusPublished {
				anyPublished = true
				if r.PostID != "" {
					publishedIds[r.Platform] = r.PostID
				}
			}
			if r.Status == pubStatusFailed && firstErr == "" {
				firstErr = r.Error
			}
		}
		fields["publishedIds"] = publishedIds
		fields["publishError"] = firstErr
		if anyPublished {
			// The post is live now. Stamp the actual posting time onto both the
			// precise publish field and the calendar-placement field so the
			// calendar shows it when it really went out, not at a stale time.
			postedAt := time.Now().UnixMilli()
			fields["scheduledAt"] = postedAt
			fields["postingTimeStamp"] = postedAt
		}
	}
	if err := trendlymodels.UpdateContentFields(brandID, contentID, fields); err != nil {
		log.Printf("publishing: failed to update content %s: %v", contentID, err)
	}
}

func cloneResults(in []trendlymodels.ContentPublishResult) []trendlymodels.ContentPublishResult {
	out := make([]trendlymodels.ContentPublishResult, len(in))
	copy(out, in)
	return out
}

// buildSeedResults produces the initial per-destination result set (aligned 1:1
// with ct.Destinations): destinations to run are marked "publishing", untargeted
// ones "skipped", and — on a retry (onlySet non-empty) — destinations not being
// re-run carry their prior result forward unchanged.
func buildSeedResults(ct *trendlymodels.Content, onlySet map[string]bool) []trendlymodels.ContentPublishResult {
	retry := len(onlySet) > 0
	prior := map[string]trendlymodels.ContentPublishResult{}
	for _, r := range ct.PublishResults {
		prior[r.SocialAccountID] = r
	}
	results := make([]trendlymodels.ContentPublishResult, 0, len(ct.Destinations))
	for _, dest := range ct.Destinations {
		base := trendlymodels.ContentPublishResult{
			SocialAccountID: dest.SocialAccountID,
			Platform:        string(dest.Platform),
			Username:        dest.Username,
		}
		switch {
		case len(ct.Platforms) > 0 && !platformTargeted(dest.Platform, ct.Platforms):
			base.Status = pubStatusSkipped
			base.Error = "platform not targeted by this content"
		case retry && !onlySet[dest.SocialAccountID]:
			if p, ok := prior[dest.SocialAccountID]; ok {
				base = p
			} else {
				base.Status = pubStatusSkipped
			}
		default:
			base.Status = pubStatusPublishing
		}
		results = append(results, base)
	}
	return results
}

// SeedPublishing flips the targeted destinations to "publishing" and the doc to
// the "publishing" status the instant Publish/Retry is requested — before the
// queued worker starts — so the brand app reflects the in-flight state without
// waiting on SQS delivery. `only` limits the seed to specific destinations
// (retry). Best-effort: a seed failure doesn't block the enqueue.
func SeedPublishing(brandID, contentID string, only ...string) error {
	ct, err := trendlymodels.GetContent(brandID, contentID)
	if err != nil {
		return err
	}
	if len(ct.Destinations) == 0 {
		return fmt.Errorf("content %s has no destinations", contentID)
	}
	onlySet := map[string]bool{}
	for _, id := range only {
		if id != "" {
			onlySet[id] = true
		}
	}
	writePublishState(brandID, contentID, buildSeedResults(ct, onlySet), false)
	return nil
}

// PublishContent loads a content doc and publishes it to each destination
// CONCURRENTLY, recording a per-destination result (published / failed with a
// reason) and a derived overall status on the document. One platform failing
// never blocks or aborts the others — partial success is a first-class outcome.
//
// When `only` is non-empty it names the destination socialAccountIds to (re)run
// — used by Retry to republish just the failed socials while carrying every
// other destination's prior result forward untouched.
func PublishContent(brandID, contentID string, only ...string) error {
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

	onlySet := map[string]bool{}
	for _, id := range only {
		if id != "" {
			onlySet[id] = true
		}
	}

	// results is aligned 1:1 with ct.Destinations; toRun holds the indices we
	// actually publish this run (everything marked "publishing" by the seed).
	results := buildSeedResults(ct, onlySet)
	toRun := []int{}
	for i, r := range results {
		if r.Status == pubStatusPublishing {
			toRun = append(toRun, i)
		}
	}

	// Seed the "publishing" state so the app shows spinners immediately.
	writePublishState(brandID, contentID, cloneResults(results), false)

	// Publish every destination concurrently. Completions are funnelled back to
	// THIS goroutine over `done`, which writes each progress snapshot in order —
	// a single writer keeps Firestore updates ordered and guarantees the final
	// write lands last (so a late progress write can't revert a resolved row).
	var mu sync.Mutex
	done := make(chan int, len(toRun))
	var wg sync.WaitGroup
	for _, idx := range toRun {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			dest := ct.Destinations[idx]
			id, perr := publishDestination(brandID, ct, dest, variations[string(dest.Platform)])
			mu.Lock()
			r := results[idx]
			r.At = time.Now().UnixMilli()
			if perr != nil {
				r.Status = pubStatusFailed
				r.Error = perr.Error()
				r.ErrorKind = classifyPublishError(perr)
			} else {
				r.Status = pubStatusPublished
				r.PostID = id
				r.Error = ""
				r.ErrorKind = ""
			}
			results[idx] = r
			mu.Unlock()
			done <- idx
		}(idx)
	}
	go func() {
		wg.Wait()
		close(done)
	}()
	for range done {
		mu.Lock()
		snapshot := cloneResults(results)
		mu.Unlock()
		writePublishState(brandID, contentID, snapshot, false)
	}

	// Authoritative final write: derived status + publishedIds + posting time.
	writePublishState(brandID, contentID, cloneResults(results), true)

	for _, r := range results {
		if r.Status == pubStatusFailed {
			return fmt.Errorf("publish failed for %s: %s", r.Platform, r.Error)
		}
	}
	return nil
}
