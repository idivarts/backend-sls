package reddit

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

// authedRequest builds an HTTP request against the OAuth API host with the
// required Bearer token and unique User-Agent. body may be nil. When body is
// non-nil the caller should set Content-Type.
func authedRequest(method, path, accessToken string, body io.Reader) (*http.Request, error) {
	u := path
	if strings.HasPrefix(path, "/") {
		u = APIURL + path
	}
	req, err := http.NewRequest(method, u, body)
	if err != nil {
		return nil, fmt.Errorf("reddit: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", UserAgent)
	return req, nil
}

// doAuthed performs an authed request and returns the raw body, erroring on a
// non-2xx status. Reddit also surfaces some errors inside a 200 body (see the
// json.errors field) — callers that submit content must additionally inspect
// the parsed payload.
func doAuthed(req *http.Request) ([]byte, int, error) {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("reddit: request failed: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return b, resp.StatusCode, fmt.Errorf("reddit: %s returned %d: %s", req.URL.Path, resp.StatusCode, string(b))
	}
	return b, resp.StatusCode, nil
}
