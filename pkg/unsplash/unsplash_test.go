package unsplash

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestIsStockURLMatchesTheHostNotTheString(t *testing.T) {
	// This decides whether a LICENCE restriction applies, so a prefix or
	// substring test is not good enough: an attacker-controlled URL that merely
	// mentions the host would be waved through as restricted-or-not by accident.
	stock := []string{
		"https://images.unsplash.com/photo-123?w=1080",
		"https://unsplash.com/photos/abc",
		"https://plus.unsplash.com/premium_photo-1",
		"HTTPS://IMAGES.UNSPLASH.COM/photo-123",
	}
	for _, u := range stock {
		if !IsStockURL(u) {
			t.Errorf("%q should be recognised as stock", u)
		}
	}

	notStock := []string{
		"https://d1u20zdoqkyuuq.cloudfront.net/uploads/file_1.png",
		"https://evil.example.com/?x=images.unsplash.com",
		"https://images.unsplash.com.evil.example.com/photo",
		"https://notunsplash.com/photo",
		"",
		"::not a url::",
	}
	for _, u := range notStock {
		if IsStockURL(u) {
			t.Errorf("%q must NOT be treated as stock", u)
		}
	}
}

func TestReferralTagsLinksWithoutLosingTheirQuery(t *testing.T) {
	got := referral("https://unsplash.com/@someone?foo=bar")
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	q := u.Query()
	if q.Get("utm_source") != "trendly" || q.Get("utm_medium") != "referral" {
		t.Errorf("referral params missing: %s", got)
	}
	if q.Get("foo") != "bar" {
		t.Errorf("existing query dropped: %s", got)
	}
	if referral("") != "" {
		t.Error("an empty link should stay empty")
	}
}

func TestTrackDownloadRefusesAForeignLocation(t *testing.T) {
	t.Setenv("UNSPLASH_ACCESS_KEY", "test-key")
	// The location arrives inside a JSON response. A request that carries our
	// Authorization header must never be aimed by one at a host we did not
	// choose.
	err := TrackDownload(context.Background(), "https://evil.example.com/collect")
	if err == nil || !strings.Contains(err.Error(), "refusing to ping") {
		t.Errorf("want a refusal, got %v", err)
	}
	if err := TrackDownload(context.Background(), ""); err == nil {
		t.Error("an empty location should be refused")
	}
}

func TestTrackDownloadFiresTheRequest(t *testing.T) {
	t.Setenv("UNSPLASH_ACCESS_KEY", "test-key")
	var hits int
	var auth, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		auth = r.Header.Get("Authorization")
		path = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	old := pingPrefix
	pingPrefix = srv.URL + "/"
	defer func() { pingPrefix = old }()

	if err := TrackDownload(context.Background(), srv.URL+"/photos/abc/download"); err != nil {
		t.Fatalf("track: %v", err)
	}
	if hits != 1 {
		t.Errorf("the ping must be sent exactly once, got %d", hits)
	}
	if auth != "Client-ID test-key" {
		t.Errorf("auth header = %q", auth)
	}
	if path != "/photos/abc/download" {
		t.Errorf("pinged %q", path)
	}
}

func TestSearchRequiresConfiguration(t *testing.T) {
	t.Setenv("UNSPLASH_ACCESS_KEY", "")
	if _, err := Search(context.Background(), "coffee", "", 1); err != ErrNotConfigured {
		t.Errorf("want ErrNotConfigured, got %v", err)
	}
	if IsConfigured() {
		t.Error("IsConfigured should be false with no key")
	}
}

func TestSearchParsesAPhotoIntoWhatThePlacementNeeds(t *testing.T) {
	t.Setenv("UNSPLASH_ACCESS_KEY", "test-key")

	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		if got := r.Header.Get("Authorization"); got != "Client-ID test-key" {
			t.Errorf("auth header = %q", got)
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{
			"id":"abc",
			"alt_description":"a cat on a desk",
			"urls":{"raw":"https://images.unsplash.com/raw","full":"https://images.unsplash.com/full","regular":"https://images.unsplash.com/regular","small":"https://images.unsplash.com/small"},
			"links":{"html":"https://unsplash.com/photos/abc","download_location":"https://api.unsplash.com/photos/abc/download"},
			"user":{"name":"Ada","links":{"html":"https://unsplash.com/@ada"}}
		}]}`))
	}))
	defer srv.Close()

	old := searchURL
	searchURL = srv.URL
	defer func() { searchURL = old }()

	photos, err := Search(context.Background(), "cat", OrientationPortrait, 3)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(photos) != 1 {
		t.Fatalf("got %d photos", len(photos))
	}
	p := photos[0]

	// `regular` and not `full`: the terms expect the returned URLs to be used,
	// and regular is the right size for a slide without being megabytes.
	if p.URL != "https://images.unsplash.com/regular" {
		t.Errorf("url = %q, want the regular size", p.URL)
	}
	if p.Alt != "a cat on a desk" {
		t.Errorf("alt = %q", p.Alt)
	}
	if p.Photographer != "Ada" {
		t.Errorf("photographer = %q", p.Photographer)
	}
	// The credit link must carry the referral tag; the download location must
	// NOT be rewritten, because it is handed straight back to the API.
	if !strings.Contains(p.PhotographerURL, "utm_source=trendly") ||
		!strings.Contains(p.PhotographerURL, "utm_medium=referral") {
		t.Errorf("photographer link is not referral-tagged: %q", p.PhotographerURL)
	}
	if !strings.Contains(p.PageURL, "utm_source=trendly") {
		t.Errorf("page link is not referral-tagged: %q", p.PageURL)
	}
	if p.DownloadLocation != "https://api.unsplash.com/photos/abc/download" {
		t.Errorf("download location was rewritten: %q", p.DownloadLocation)
	}

	// "squarish" is Unsplash's word; sending Pexels' "square" would silently
	// return unfiltered results rather than an error.
	if gotQuery.Get("orientation") != "portrait" {
		t.Errorf("orientation = %q", gotQuery.Get("orientation"))
	}
	if gotQuery.Get("per_page") != "3" {
		t.Errorf("per_page = %q", gotQuery.Get("per_page"))
	}
	if gotQuery.Get("content_filter") != "high" {
		t.Errorf("content_filter = %q — these land in published posts", gotQuery.Get("content_filter"))
	}
	if gotQuery.Get("query") != "cat" {
		t.Errorf("query = %q", gotQuery.Get("query"))
	}
}

func TestSearchSkipsAResultWithNoUsableImage(t *testing.T) {
	t.Setenv("UNSPLASH_ACCESS_KEY", "test-key")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"id":"x","urls":{},"links":{},"user":{}}]}`))
	}))
	defer srv.Close()
	old := searchURL
	searchURL = srv.URL
	defer func() { searchURL = old }()

	photos, err := Search(context.Background(), "cat", "", 1)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(photos) != 0 {
		t.Errorf("a result with no image url should be dropped, got %+v", photos)
	}
}

func TestSearchNamesTheDemoRateLimit(t *testing.T) {
	t.Setenv("UNSPLASH_ACCESS_KEY", "test-key")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	old := searchURL
	searchURL = srv.URL
	defer func() { searchURL = old }()

	_, err := Search(context.Background(), "cat", "", 1)
	// A 403 here is nearly always Demo mode (50 req/hour), not a bad key, and
	// saying so is the difference between "apply for production access" and a
	// hunt for a credential bug.
	if err == nil || !strings.Contains(err.Error(), "Demo") {
		t.Errorf("a 403 should name Demo mode, got %v", err)
	}
}
