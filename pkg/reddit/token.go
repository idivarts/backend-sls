package reddit

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

// TokenResponse is returned by Reddit's access_token endpoint. A refresh token
// is only issued when the authorize request used duration=permanent.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int    `json:"expires_in"` // typically 3600 (1 hour)
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
}

// ExchangeCode exchanges an authorization code for access + refresh tokens.
// Reddit is a confidential client: it requires HTTP Basic auth (clientID:secret)
// and a unique User-Agent.
func ExchangeCode(code, redirectURI string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	return doTokenRequest(data)
}

// RefreshAccessToken exchanges a refresh token for a new access token. Reddit
// refresh tokens do not rotate (the same refresh token keeps working).
func RefreshAccessToken(refreshToken string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)
	return doTokenRequest(data)
}

func doTokenRequest(data url.Values) (*TokenResponse, error) {
	req, err := http.NewRequest(http.MethodPost, TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("reddit: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Basic "+basicAuth(ClientID, ClientSecret))
	req.Header.Set("User-Agent", UserAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reddit: token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reddit: token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var t TokenResponse
	if err := json.Unmarshal(body, &t); err != nil {
		return nil, fmt.Errorf("reddit: parse token response: %w", err)
	}
	if t.AccessToken == "" {
		return nil, fmt.Errorf("reddit: token response missing access_token: %s", string(body))
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
