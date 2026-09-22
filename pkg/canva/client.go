// Package canva is a server-side client for the Canva Connect API (OAuth 2.0 +
// PKCE, assets, designs, exports, design import, and return-navigation JWT
// verification). Token exchange is backend-only (client secret + Basic auth);
// the RN client never sees Canva credentials.
//
// NOTE: endpoint field names follow the Canva Connect REST v1 reference. Verify
// against https://www.canva.dev/docs/connect/ when enabling the live
// integration — see backend-sls/docs/canva-setup.md.
package canva

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	authorizeURL = "https://www.canva.com/api/oauth/authorize"
	apiBase      = "https://api.canva.com/rest/v1"
	// jwksURL serves the public keys used to verify return-navigation JWTs.
	jwksURL = "https://api.canva.com/rest/v1/connect/keys"
)

// DefaultScopes requested during OAuth. asset:* for uploading our drafts,
// design:* for create/import/export, profile:read for identity.
const DefaultScopes = "asset:read asset:write design:content:read design:content:write design:meta:read profile:read"

var (
	clientID     = os.Getenv("CANVA_CLIENT_ID")
	clientSecret = os.Getenv("CANVA_CLIENT_SECRET")
	// redirectURI must exactly match one configured on the Canva integration.
	redirectURI = os.Getenv("CANVA_REDIRECT_URI")
	httpClient  = &http.Client{Timeout: 60 * time.Second}
)

// Configured reports whether the Canva credentials are present.
func Configured() bool { return clientID != "" && clientSecret != "" }

// RedirectURI exposes the configured OAuth redirect for handlers.
func RedirectURI() string { return redirectURI }

// apiError decodes a Canva error body for surfacing.
type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func jsonUnmarshal(b []byte, v interface{}) error { return json.Unmarshal(b, v) }

// doJSON performs an authenticated JSON request against the Connect API.
func doJSON(method, path, accessToken string, body interface{}, out interface{}) error {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("canva marshal: %w", err)
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, apiBase+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("canva %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var ae apiError
		_ = json.Unmarshal(raw, &ae)
		return fmt.Errorf("canva %s %s returned %s: %s %s", method, path, resp.Status, ae.Code, ae.Message)
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("canva decode %s: %w", path, err)
		}
	}
	return nil
}
