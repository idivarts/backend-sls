package youtube

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// TokenResponse is returned by Google's token endpoint.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"` // only on first exchange
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
}

// ExchangeCode exchanges an authorization code for access + refresh tokens.
// redirectURI must match the one registered in Google Cloud Console.
func ExchangeCode(code, redirectURI string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("code", code)
	data.Set("client_id", ClientID)
	data.Set("client_secret", ClientSecret)
	data.Set("redirect_uri", redirectURI)
	data.Set("grant_type", "authorization_code")

	resp, err := http.Post(TokenURL, "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("youtube: token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("youtube: token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var t TokenResponse
	if err := json.Unmarshal(body, &t); err != nil {
		return nil, fmt.Errorf("youtube: failed to parse token response: %w", err)
	}
	return &t, nil
}

// RefreshAccessToken uses a stored refresh token to obtain a new access token.
// Google does not rotate refresh tokens unless access is revoked; the returned
// TokenResponse will have an empty RefreshToken field — keep the original.
func RefreshAccessToken(refreshToken string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("refresh_token", refreshToken)
	data.Set("client_id", ClientID)
	data.Set("client_secret", ClientSecret)
	data.Set("grant_type", "refresh_token")

	resp, err := http.Post(TokenURL, "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("youtube: refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("youtube: refresh endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var t TokenResponse
	if err := json.Unmarshal(body, &t); err != nil {
		return nil, fmt.Errorf("youtube: failed to parse refresh response: %w", err)
	}
	return &t, nil
}

// ExpiresAt converts the ExpiresIn (seconds from now) to an absolute Unix timestamp.
func (t *TokenResponse) ExpiresAt() int64 {
	return time.Now().Add(time.Duration(t.ExpiresIn) * time.Second).Unix()
}
