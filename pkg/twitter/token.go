package twitter

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// TokenResponse is returned by Twitter's token endpoint.
// Refresh tokens are only issued when offline.access scope is requested.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int    `json:"expires_in"` // typically 7200 (2 hours)
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
}

// ExchangeCode exchanges a PKCE authorization code for access + refresh tokens.
// codeVerifier is the plain-text PKCE verifier that was used to generate the
// code_challenge sent in the authorization request.
func ExchangeCode(code, redirectURI, codeVerifier string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	data.Set("code_verifier", codeVerifier)

	req, err := http.NewRequest(http.MethodPost, TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("twitter: failed to build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Twitter requires Basic auth for confidential clients
	req.Header.Set("Authorization", "Basic "+basicAuth(ClientID, ClientSecret))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitter: token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitter: token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var t TokenResponse
	if err := json.Unmarshal(body, &t); err != nil {
		return nil, fmt.Errorf("twitter: failed to parse token response: %w", err)
	}
	return &t, nil
}

// RefreshAccessToken exchanges a refresh token for a new access token.
// Twitter rotates refresh tokens — always persist the new refresh token.
func RefreshAccessToken(refreshToken string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)

	req, err := http.NewRequest(http.MethodPost, TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("twitter: failed to build refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Basic "+basicAuth(ClientID, ClientSecret))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitter: refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitter: refresh endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var t TokenResponse
	if err := json.Unmarshal(body, &t); err != nil {
		return nil, fmt.Errorf("twitter: failed to parse refresh response: %w", err)
	}
	return &t, nil
}

// ExpiresAt returns the absolute Unix timestamp when the access token expires.
func (t *TokenResponse) ExpiresAt() int64 {
	return time.Now().Add(time.Duration(t.ExpiresIn) * time.Second).Unix()
}

func basicAuth(clientID, clientSecret string) string {
	return base64.StdEncoding.EncodeToString([]byte(clientID + ":" + clientSecret))
}
