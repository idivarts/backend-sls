// Package unsplash is a thin client for Unsplash photo search.
//
// It exists because Pexels paused new API key issuance around 30 September
// 2026, which left the stock-photography path shipped, correct and dormant
// behind a key that can no longer be self-served. Pexels is NOT removed — its
// driver stays registered, because "paused" is not "gone" and a reopened key
// should be a configuration change rather than a rewrite.
//
// ── Three of Unsplash's terms are code, not notes ───────────────────────────
// This package is shaped by them, and each one reverses or tightens something
// the Pexels path did:
//
//  1. HOTLINK, DO NOT RE-HOST. API Guideline 1 and §6 require an app to use the
//     image URLs the API returns, and call failure to do so a material breach.
//     So Search returns urls.regular to be used as the src directly. The caller
//     must not copy the bytes to our own storage, which is what it does for
//     generated images.
//
//  2. THE DOWNLOAD PING IS MANDATORY. Guideline 2 and §6: when an app does
//     "something similar to a download" — a user choosing a photo for a post —
//     it must request the photo's download_location. That is a PLACEMENT event,
//     not a search event, so TrackDownload is deliberately separate from Search
//     and is fired once per photo actually used.
//
//  3. API CONTENT MAY NOT FEED A MODEL. §12 routes any use of API content "in
//     connection with any machine learning and/or artificial intelligence
//     purposes" to a separate agreement. Handing one of these photos to an
//     image model as a reference is squarely that use, so IsStockURL exists to
//     let the generation path REFUSE it rather than trust a comment.
//
// Point 3 is also why hotlinking turns out to be convenient as well as
// required: because the photo keeps its unsplash.com host, its provenance is
// carried by the URL itself. Had we re-hosted, every stock photo would be
// indistinguishable from the user's own uploads and the gate would need a
// side-channel to work at all.
package unsplash

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// ErrNotConfigured means no access key is present. Callers treat this as "skip
// stock photography", never as a failure — a design without a stock photo is
// still a design, and a missing key is an ops gap, not a user error.
var ErrNotConfigured = errors.New("unsplash: UNSPLASH_ACCESS_KEY is not set")

// searchURL is a var, not a const, so a test can point the client at a local
// server and exercise the response parsing — which is where the orientation
// vocabulary, the url size choice and the referral tagging all live.
var searchURL = "https://api.unsplash.com/search/photos"

// pingPrefix is the only prefix TrackDownload will send our key to. A var for
// the same reason as searchURL: the guard and the request it guards both
// deserve a test, and one that can only assert the refusal is half a test.
var pingPrefix = "https://api.unsplash.com/"

const (
	timeout = 12 * time.Second

	// utmSource/utmMedium are required on every link back. §9 and Guideline 3:
	// the credit must link to the photographer's profile with this referral
	// tag, which is how Unsplash attributes traffic to the app.
	utmSource = "trendly"
	utmMedium = "referral"
)

// Photo is the subset of an Unsplash result this product uses.
type Photo struct {
	ID string
	// URL is the image to HOTLINK. It is used as the src as-is; copying it to
	// our own storage would breach the API terms. See the package note.
	URL string
	// Alt is Unsplash's own description, used as the img alt text.
	Alt string
	// Photographer and PhotographerURL are the required credit. The URL already
	// carries the referral parameters.
	Photographer    string
	PhotographerURL string
	// PageURL is the photo's page on Unsplash, referral-tagged.
	PageURL string
	// DownloadLocation is the endpoint that MUST be requested when this photo
	// is actually placed. Opaque — hand it back to TrackDownload.
	DownloadLocation string
}

// Orientation values the API accepts. Anything else is sent as no filter.
//
// Note "squarish", not "square": Unsplash's vocabulary differs from Pexels',
// and sending the wrong word silently returns unfiltered results rather than an
// error — which reads as "orientation does not work" long after the fact.
const (
	OrientationPortrait  = "portrait"
	OrientationLandscape = "landscape"
	OrientationSquare    = "squarish"
)

func accessKey() string { return strings.TrimSpace(os.Getenv("UNSPLASH_ACCESS_KEY")) }

// IsConfigured reports whether stock search can run at all.
func IsConfigured() bool { return accessKey() != "" }

// stockHosts are the hosts a photo served by this package can come from.
//
// This is the provenance check that keeps API content out of a model's input.
// It is a HOST match rather than a prefix match on purpose: a prefix test is
// defeated by "https://evil.example.com/?x=images.unsplash.com", and this
// decides whether a licence restriction applies.
var stockHosts = map[string]bool{
	"images.unsplash.com": true,
	"unsplash.com":        true,
	"www.unsplash.com":    true,
	"plus.unsplash.com":   true,
}

// IsStockURL reports whether a URL points at Unsplash-served media.
//
// Used to refuse such a photo as a generation reference. Anything unparseable
// is NOT treated as stock: the caller's job is to block a known-restricted
// source, and failing closed on every malformed string would block ordinary
// uploads instead.
func IsStockURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return false
	}
	return stockHosts[strings.ToLower(u.Host)]
}

// referral appends the required utm parameters to an unsplash.com link.
func referral(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	q.Set("utm_source", utmSource)
	q.Set("utm_medium", utmMedium)
	u.RawQuery = q.Encode()
	return u.String()
}

type apiResponse struct {
	Results []struct {
		ID             string `json:"id"`
		AltDescription string `json:"alt_description"`
		Description    string `json:"description"`
		URLs           struct {
			Raw     string `json:"raw"`
			Full    string `json:"full"`
			Regular string `json:"regular"`
			Small   string `json:"small"`
		} `json:"urls"`
		Links struct {
			HTML             string `json:"html"`
			DownloadLocation string `json:"download_location"`
		} `json:"links"`
		User struct {
			Name  string `json:"name"`
			Links struct {
				HTML string `json:"html"`
			} `json:"links"`
		} `json:"user"`
	} `json:"results"`
}

// Search returns up to `count` photos matching the query.
//
// `orientation` is one of the Orientation constants, or "" for no filter.
func Search(ctx context.Context, query, orientation string, count int) ([]Photo, error) {
	key := accessKey()
	if key == "" {
		return nil, ErrNotConfigured
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("unsplash: empty query")
	}
	if count < 1 {
		count = 1
	}
	if count > 30 {
		count = 30
	}

	q := url.Values{}
	q.Set("query", query)
	q.Set("per_page", fmt.Sprint(count))
	// content_filter=high: these land in a brand's published post, where a
	// borderline result is a far worse outcome than a thinner result set.
	q.Set("content_filter", "high")
	switch orientation {
	case OrientationPortrait, OrientationLandscape, OrientationSquare:
		q.Set("orientation", orientation)
	}

	var body apiResponse
	if err := get(ctx, searchURL+"?"+q.Encode(), key, &body); err != nil {
		return nil, err
	}

	out := make([]Photo, 0, len(body.Results))
	for _, p := range body.Results {
		// regular (~1080px wide) is what the terms expect an app to use, and it
		// is the right size for a slide anyway: `full` is many megabytes of
		// wasted bytes and `small` is visibly soft once it fills a frame.
		src := firstNonEmpty(p.URLs.Regular, p.URLs.Full, p.URLs.Raw, p.URLs.Small)
		if src == "" {
			continue
		}
		out = append(out, Photo{
			ID:               p.ID,
			URL:              src,
			Alt:              firstNonEmpty(p.AltDescription, p.Description),
			Photographer:     p.User.Name,
			PhotographerURL:  referral(p.User.Links.HTML),
			PageURL:          referral(p.Links.HTML),
			DownloadLocation: p.Links.DownloadLocation,
		})
	}
	return out, nil
}

// TrackDownload reports that a photo has actually been USED.
//
// Required by Guideline 2 whenever the app does "something similar to a
// download" — here, resolving a data-stock placeholder into a real design.
// Called once per photo placed, never once per search result: pinging every
// result would misreport which photographers are actually being used, which is
// what the endpoint exists to measure.
//
// The response body is deliberately ignored; only the request matters.
func TrackDownload(ctx context.Context, downloadLocation string) error {
	key := accessKey()
	if key == "" {
		return ErrNotConfigured
	}
	if strings.TrimSpace(downloadLocation) == "" {
		return errors.New("unsplash: empty download location")
	}
	// Only Unsplash's own endpoint may be pinged with our key attached. The
	// location arrives inside a JSON response, and a request carrying an
	// Authorization header must never be aimed by one at a host we did not
	// choose.
	if !strings.HasPrefix(downloadLocation, pingPrefix) {
		return fmt.Errorf("unsplash: refusing to ping %q", downloadLocation)
	}
	return get(ctx, downloadLocation, key, nil)
}

// get performs an authorised GET, decoding into `out` when one is given.
func get(ctx context.Context, endpoint, key string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Client-ID "+key)
	req.Header.Set("Accept-Version", "v1")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// 403 here is nearly always the Demo rate limit (50/hour) rather than a
		// bad key, so it is named: an app stays in Demo until Unsplash approves
		// it for production, and that is an application to make, not a bug.
		if resp.StatusCode == http.StatusForbidden {
			return fmt.Errorf("unsplash: 403 — rate limited, or the app is still in Demo mode")
		}
		return fmt.Errorf("unsplash: request returned %d", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
