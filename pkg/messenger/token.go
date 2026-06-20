package messenger

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
)

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in,omitempty"` // seconds; present on long-lived token response
}

// CodeTokenResponse is returned when exchanging an authorization code.
type CodeTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in,omitempty"`
	UserID      string `json:"user_id,omitempty"` // not returned by FB; requires a /me call
}

// GetAccessTokenFromCode exchanges a server-side authorization code for a
// short-lived user access token. redirectURI must exactly match the one
// registered in your Facebook app and used in the authorization request.
func GetAccessTokenFromCode(code, redirectURI string) (*CodeTokenResponse, error) {
	endpoint := fmt.Sprintf("%s/%s/oauth/access_token", BaseURL, ApiVersion)
	data := url.Values{}
	data.Set("client_id", ClientID)
	data.Set("client_secret", ClientSecret)
	data.Set("redirect_uri", redirectURI)
	data.Set("code", code)

	resp, err := http.Post(endpoint, "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("messenger: code exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("messenger: unexpected status " + resp.Status + ": " + string(b))
	}

	log.Println("FB code exchange response:", string(b))
	var token CodeTokenResponse
	if err := json.Unmarshal(b, &token); err != nil {
		return nil, err
	}
	return &token, nil
}

// GetMe returns the Facebook user ID and name for the authenticated user.
func GetMeID(accessToken string) (string, error) {
	u := fmt.Sprintf("%s/%s/me?fields=id&access_token=%s", BaseURL, ApiVersion, accessToken)
	resp, err := http.Get(u)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", errors.New("messenger: /me returned " + resp.Status + ": " + string(b))
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(b, &result); err != nil {
		return "", err
	}
	return result.ID, nil
}

func GetLongLivedAccessToken(accessToken string) (*TokenResponse, error) {
	url := fmt.Sprintf("%s/%s/oauth/access_token?grant_type=fb_exchange_token&client_id=%s&client_secret=%s&fb_exchange_token=%s", BaseURL, ApiVersion, ClientID, ClientSecret, accessToken)

	resp, err := http.Get(url)
	if err != nil {
		fmt.Println("Error making GET request:", err)
		return nil, err
	}

	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("Error: Unexpected status code - " + resp.Status + "\n" + string(b))
	}

	log.Println("Token Url output", string(b))
	token := &TokenResponse{}
	err = json.Unmarshal(b, token)
	if err != nil {
		return nil, err
	}
	return token, nil
}
