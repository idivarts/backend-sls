package branch

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestCreateLinkLive exercises the real Branch API. It is skipped unless
// BRANCH_BRAND_KEY is set, so CI and `go test ./...` stay offline; run it against
// the TEST Branch app when changing the request shape, since a malformed `data`
// dict is accepted-but-ignored by Branch rather than rejected:
//
//	BRANCH_BRAND_KEY=key_test_… go test ./pkg/branch -run Live -v
func TestCreateLinkLive(t *testing.T) {
	key := os.Getenv("BRANCH_BRAND_KEY")
	if key == "" {
		t.Skip("BRANCH_BRAND_KEY not set — skipping live Branch API test")
	}
	if strings.HasPrefix(key, "key_live_") {
		t.Skip("refusing to create links against the LIVE Branch app")
	}

	creds := Credentials{Key: key, Secret: os.Getenv("BRANCH_BRAND_SECRET")}
	url, err := creds.CreateLink(context.Background(), LinkRequest{
		Channel: "app-share",
		Feature: "public-share",
		Stage:   "strategy",
		Type:    TypeDefault,
		Data: LinkData{
			OGTitle:       "Q4 content strategy",
			OGDescription: "A shared content strategy, read-only.",
			DeeplinkPath:  "/share/livetest-token",
			DesktopURL:    "https://dev.brands.trendly.now/share/livetest-token",
			FallbackURL:   "https://dev.brands.trendly.now/share/livetest-token",
			Extra: map[string]string{
				"shareToken": "livetest-token",
				"shareType":  "strategy",
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateLink: %v", err)
	}
	if !strings.Contains(url, "test-app.link") {
		t.Errorf("url = %q, want a test-app.link host", url)
	}
	t.Logf("created %s", url)

	// Read it back to confirm Branch stored the control params we sent, rather
	// than silently dropping them.
	if creds.Secret == "" {
		t.Skip("no BRANCH_BRAND_SECRET — skipping read-back")
	}
	got, err := creds.ReadLink(context.Background(), url)
	if err != nil {
		t.Fatalf("ReadLink: %v", err)
	}
	data, _ := got["data"].(map[string]interface{})
	if data == nil {
		// Branch returns `data` as a JSON *string* on some link shapes.
		t.Logf("read back: %+v", got)
		return
	}
	if data["$deeplink_path"] != "/share/livetest-token" {
		t.Errorf("$deeplink_path = %v, want /share/livetest-token", data["$deeplink_path"])
	}
	if data["shareToken"] != "livetest-token" {
		t.Errorf("shareToken = %v, want livetest-token", data["shareToken"])
	}
}
