package linkedin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// TokenResponse is returned by LinkedIn's token endpoint.
// LinkedIn access tokens expire after 60 days; refresh tokens after 1 year.
type TokenResponse struct {
	AccessToken           string `json:"access_token"`
	ExpiresIn             int    `json:"expires_in"`              // seconds (~5184000 = 60 days)
	RefreshToken          string `json:"refresh_token,omitempty"` // only when offline_access scope requested
	RefreshTokenExpiresIn int    `json:"refresh_token_expires_in,omitempty"`
	TokenType             string `json:"token_type"`
	Scope                 string `json:"scope"`
}

// ExchangeCode exchanges an authorization code for an access token.
func ExchangeCode(code, redirectURI string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	data.Set("client_id", ClientID)
	data.Set("client_secret", ClientSecret)

	resp, err := http.Post(TokenURL, "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("linkedin: token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("linkedin: token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var t TokenResponse
	if err := json.Unmarshal(body, &t); err != nil {
		return nil, fmt.Errorf("linkedin: failed to parse token response: %w", err)
	}
	return &t, nil
}

// RefreshAccessToken uses a refresh token to obtain a new access token.
// Note: LinkedIn only issues refresh tokens when offline_access scope is requested.
func RefreshAccessToken(refreshToken string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)
	data.Set("client_id", ClientID)
	data.Set("client_secret", ClientSecret)

	resp, err := http.Post(TokenURL, "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("linkedin: refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("linkedin: refresh endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var t TokenResponse
	if err := json.Unmarshal(body, &t); err != nil {
		return nil, fmt.Errorf("linkedin: failed to parse refresh response: %w", err)
	}
	return &t, nil
}

// ExpiresAt returns the absolute Unix timestamp when the access token expires.
func (t *TokenResponse) ExpiresAt() int64 {
	return time.Now().Add(time.Duration(t.ExpiresIn) * time.Second).Unix()
}
