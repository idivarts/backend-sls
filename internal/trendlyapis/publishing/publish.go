package publishing

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/instagram"
	"github.com/idivarts/backend-sls/pkg/linkedin"
	"github.com/idivarts/backend-sls/pkg/messenger"
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

	var res *messenger.FBPublishResponse
	var err error
	if img != "" {
		res, err = messenger.PublishPagePhoto(pageID, img, caption, pageToken)
	} else {
		res, err = messenger.PublishPageFeed(pageID, caption, "", pageToken)
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
	return linkedin.CreateMemberPost(accessToken, authorURN, buildCaption(ct), firstImageURL(ct))
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

	publishedIds := map[string]string{}
	var firstErr error

	for _, dest := range ct.Destinations {
		// Never publish to a platform the content isn't targeting. (Legacy docs
		// with no `platforms` set skip this guard.)
		if len(ct.Platforms) > 0 && !platformTargeted(dest.Platform, ct.Platforms) {
			log.Printf("publishing: destination platform %q not in content %s targeted platforms; skipping", dest.Platform, contentID)
			continue
		}
		account, aerr := trendlymodels.GetBrandSocialAccount(brandID, dest.SocialAccountID)
		if aerr != nil {
			if firstErr == nil {
				firstErr = aerr
			}
			continue
		}
		token, terr := trendlymodels.GetBrandSocialToken(brandID, dest.SocialAccountID)
		if terr != nil {
			if firstErr == nil {
				firstErr = terr
			}
			continue
		}

		switch dest.Platform {
		case "instagram":
			id, perr := publishToInstagram(account.PlatformAccountID, token.AccessToken, ct)
			if perr != nil {
				if firstErr == nil {
					firstErr = perr
				}
				continue
			}
			publishedIds["instagram"] = id
		case "facebook":
			id, perr := publishToFacebook(account.PlatformAccountID, token.AccessToken, ct)
			if perr != nil {
				if firstErr == nil {
					firstErr = perr
				}
				continue
			}
			publishedIds["facebook"] = id
		case "linkedin":
			id, perr := publishToLinkedIn(account, token.AccessToken, ct)
			if perr != nil {
				if firstErr == nil {
					firstErr = perr
				}
				continue
			}
			publishedIds["linkedin"] = id
		default:
			log.Printf("publishing: unsupported platform %q for content %s", dest.Platform, contentID)
		}
	}

	fields := map[string]interface{}{
		"publishedIds": publishedIds,
	}
	if len(publishedIds) > 0 {
		fields["status"] = "posted"
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
