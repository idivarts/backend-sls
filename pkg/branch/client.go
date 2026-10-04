// Package branch is a server-side client for the Branch Deep Linking API —
// creating, reading, updating and revoking Branch deep links ("dynamic links").
//
// There is no official Branch SDK for Go: BranchMetrics ships native SDKs
// (iOS/Android/RN/Web) plus a documented REST API, and publishes no Go library.
// So this is a thin hand-rolled client over that REST API rather than a wrapper
// around a vendored SDK.
//
//	https://help.branch.io/developers-hub/reference/deep-linking-api
//
// Credentials come from the environment, per stage (test key on dev, live key on
// prod — see the BRANCH_BRAND_* GitHub Actions environment variables):
//
//	BRANCH_BRAND_KEY     key_live_… / key_test_…  — required to CREATE a link
//	BRANCH_BRAND_SECRET  secret_live_… / secret_test_…  — required to READ/UPDATE
//	BRANCH_BRAND_DOMAIN  zgh4c.app.link / zgh4c.test-app.link (informational)
//
// The key is not a secret (it is compiled into the mobile bundle); the secret is.
//
// Naming: BRANCH_BRAND_* is the brand app's Branch app. The influencer app gets
// its own Branch app and its own BRANCH_USER_* credentials, so a second
// Credentials value can be added here without touching call sites.
package branch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const apiBase = "https://api2.branch.io"

var httpClient = &http.Client{Timeout: 20 * time.Second}

// Credentials identifies one Branch app. Read from the environment at init so
// an unconfigured stage is detectable before any call is attempted.
type Credentials struct {
	Key    string
	Secret string
	Domain string
}

// Brand holds the credentials for the brand app's Branch app (BRANCH_BRAND_*).
var Brand = Credentials{
	Key:    os.Getenv("BRANCH_BRAND_KEY"),
	Secret: os.Getenv("BRANCH_BRAND_SECRET"),
	Domain: os.Getenv("BRANCH_BRAND_DOMAIN"),
}

// Configured reports whether links can be created for this app. Only the key is
// needed to create; the secret is required for read/update, which Configured
// deliberately does not gate — an unset secret must not disable link creation.
func (c Credentials) Configured() bool { return c.Key != "" }

// ErrNotConfigured is returned when the stage carries no Branch key, so callers
// can degrade to a plain web URL instead of failing the request.
var ErrNotConfigured = fmt.Errorf("branch: no branch_key configured for this stage")

// apiError is Branch's error body shape. Branch is inconsistent about which of
// these it populates, so surfacing captures all three.
type apiError struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
	Message string `json:"message"`
	Code    int    `json:"code"`
}

func (e apiError) text() string {
	if e.Error.Message != "" {
		return e.Error.Message
	}
	return e.Message
}

// doJSON performs a JSON request against the Branch API. Branch authenticates
// link calls by the key/secret carried in the BODY, not by a header, so there is
// no Authorization to set here.
func doJSON(ctx context.Context, method, path string, body, out interface{}) error {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("branch marshal: %w", err)
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, apiBase+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("branch %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var ae apiError
		_ = json.Unmarshal(raw, &ae)
		return fmt.Errorf("branch %s %s returned %s: %s", method, path, resp.Status, ae.text())
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("branch decode %s: %w", path, err)
		}
	}
	return nil
}
