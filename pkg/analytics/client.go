// Package analytics emits product-analytics events from the backend into
// PostHog, over its HTTP capture API.
//
// Why the backend emits events at all. Almost everything in the funnel is
// reported by the apps, which is the right default — the client knows what the
// user did. Two steps of the email signup funnel are not visible from there at
// all:
//
//   - the verification mail being SENT. /onboard/signup creates the account and
//     dispatches the mail in one handler, so a 2xx tells the app only that both
//     happened; it cannot report them apart, and it cannot see a send that
//     failed after the account was created.
//   - the email being VERIFIED. The link in the mail hits
//     GET /onboard/email-redirection, which marks the account verified and only
//     then redirects to the login page. A client-side "verified" event fired on
//     that landing would systematically miss everyone who opens the link in a
//     mail-app webview or on a device where the app intercepts it — a biased
//     slice, not random loss, which is worse than no number.
//
// So these events are emitted here, keyed on the Firebase uid as distinct_id.
// The apps call identify() with the same uid, so PostHog stitches the two sides
// into one person and the funnel reads end to end.
//
// There is no official PostHog Go SDK in this repo's dependency set, and the
// capture API is a single JSON POST, so this is a thin hand-rolled client in the
// same spirit as pkg/branch.
//
//	https://posthog.com/docs/api/capture
//
// Configuration comes from the environment, per stage:
//
//	POSTHOG_API_KEY   the project API key (same key the apps use)
//	POSTHOG_HOST      optional; defaults to PostHog US cloud
//
// An unconfigured stage makes every call a silent no-op. That is deliberate:
// analytics must never be able to fail a signup.
package analytics

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"
)

const defaultHost = "https://us.i.posthog.com"

// Short timeout: this sits in the request path of a signup, and a slow
// analytics endpoint must not become a slow signup.
var httpClient = &http.Client{Timeout: 5 * time.Second}

var (
	apiKey = os.Getenv("POSTHOG_API_KEY")
	host   = os.Getenv("POSTHOG_HOST")
)

// Configured reports whether events can be sent for this stage.
func Configured() bool { return apiKey != "" }

func baseURL() string {
	if host != "" {
		return host
	}
	return defaultHost
}

type capturePayload struct {
	APIKey     string                 `json:"api_key"`
	Event      string                 `json:"event"`
	DistinctID string                 `json:"distinct_id"`
	Properties map[string]interface{} `json:"properties,omitempty"`
	Timestamp  string                 `json:"timestamp,omitempty"`
}

// Capture sends one event for the given distinct id.
//
// `distinctID` MUST be the Firebase uid, not an email: it is the only
// identifier both sides of this funnel share, and it is what the apps pass to
// identify(). Keying on an email would also put a personal identifier in the
// analytics index for no gain.
//
// Errors are logged and swallowed rather than returned. Every caller is in the
// middle of something that matters more than the event — a signup, a
// verification — and none of them should branch on whether analytics worked.
func Capture(ctx context.Context, distinctID, event string, props map[string]interface{}) {
	if !Configured() || distinctID == "" {
		return
	}

	// The site/environment dimensions the apps set as super-properties. Set
	// here too so a backend event can be filtered the same way as a client one
	// — without them these events would be the only rows in the project with no
	// environment, and would quietly pollute prod dashboards from dev.
	all := map[string]interface{}{
		"site":        "backend",
		"environment": os.Getenv("STAGE"),
	}
	for k, v := range props {
		all[k] = v
	}

	payload := capturePayload{
		APIKey:     apiKey,
		Event:      event,
		DistinctID: distinctID,
		Properties: all,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("analytics: marshal %s: %v", event, err)
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL()+"/i/v0/e/", bytes.NewReader(body))
	if err != nil {
		log.Printf("analytics: request %s: %v", event, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("analytics: post %s: %v", event, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("analytics: %s returned %s", event, resp.Status)
	}
}

// CaptureAsync sends an event without making the caller wait for it.
//
// Used where the event is incidental to a user-facing response — a signup
// should not pay 5s of analytics latency because PostHog is slow. The context
// is deliberately NOT the request's: a Lambda cancels its request context the
// moment the handler returns, which would abort the in-flight POST. It gets its
// own short-lived one instead.
//
// Note for the Lambda runtime: a goroutine is not guaranteed to finish once the
// handler returns and the execution environment is frozen. In practice the POST
// is issued immediately and completes within the same invocation, and losing an
// occasional event is an acceptable trade for never delaying a signup. Where an
// event must not be lost, call Capture directly.
func CaptureAsync(distinctID, event string, props map[string]interface{}) {
	if !Configured() || distinctID == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		Capture(ctx, distinctID, event, props)
	}()
}

// Identify attaches person properties to a uid.
//
// $set rather than $set_once, matching the apps' setPersonProperties: the
// backend only writes properties whose latest value is the useful one.
func Identify(distinctID string, props map[string]interface{}) {
	if !Configured() || distinctID == "" {
		return
	}
	CaptureAsync(distinctID, "$identify", map[string]interface{}{
		"$set": props,
	})
}
