package facebook

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

// GraphGetRetry performs a Graph API GET with retry + backoff, shrinking the
// page size on transient failures.
//
// Meta's Conversations / Messages edges intermittently return HTTP 500 with
// error code 1 — "Please reduce the amount of data you're asking for, then retry
// your request." It is a temporary server-side error (typically tied to
// expanding participants/attachments across a page of threads) that clears on a
// retry, usually with a smaller page. urlFor must build the request URL for a
// given page size; we start at limit and halve it (down to a floor) across
// attempts so a "reduce the amount of data" 500 self-heals.
//
// Returns the response body on HTTP 200, or an error describing the last
// non-200 response once all attempts are exhausted. Non-5xx responses (e.g. a
// 4xx for a bad token or missing permission) are returned immediately — a
// smaller page or a wait will not help those.
func GraphGetRetry(urlFor func(limit int) string, limit int) ([]byte, error) {
	const (
		maxAttempts = 4
		minLimit    = 3
	)
	backoff := 400 * time.Millisecond
	var lastStatus string
	var lastBody []byte
	for attempt := 0; attempt < maxAttempts; attempt++ {
		resp, err := http.Get(urlFor(limit))
		if err != nil {
			return nil, err
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if resp.StatusCode == http.StatusOK {
			return body, nil
		}
		lastStatus, lastBody = resp.Status, body
		if resp.StatusCode < 500 {
			break
		}
		if attempt < maxAttempts-1 {
			time.Sleep(backoff)
			backoff *= 2
			if limit > minLimit {
				if limit /= 2; limit < minLimit {
					limit = minLimit
				}
			}
		}
	}
	return nil, fmt.Errorf("Error: Unexpected status code - %s\n%s", lastStatus, string(lastBody))
}
