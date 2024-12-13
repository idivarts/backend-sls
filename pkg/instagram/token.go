package instagram

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
)

type CodeResponse struct {
	AccessToken string   `json:"access_token"`
	UserID      int64    `json:"user_id"`
	Permissions []string `json:"permissions"`
}

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

func GetAccessTokenFromCode(code, redirectUri string) (*CodeResponse, error) {
	log.Println("Code is", code)

	apiURL := "https://api.instagram.com/oauth/access_token"
	data := url.Values{
		"client_id":     {ClientID},
		"client_secret": {ClientSecret},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {redirectUri},
		"code":          {code},
	}

	// Create the HTTP request
	req, err := http.NewRequest("POST", apiURL, bytes.NewBufferString(data.Encode()))
	if err != nil {
		fmt.Println("Error creating request:", err)
		return nil, err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	log.Println("Coming till here")
	// Make the HTTP request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error making request:", err)
		return nil, err
	}
	defer resp.Body.Close()

	log.Println("Request is done")
	// Read the response
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response:", err)
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("Error: Unexpected status code - " + resp.Status + "\n" + string(b))
	}

	log.Println("Token Url output", string(b))
	token := &CodeResponse{}
	err = json.Unmarshal(b, token)
	if err != nil {
		return nil, err
	}
	return token, nil
}

func GetLongLivedAccessToken(accessToken string) (*TokenResponse, error) {
	url := fmt.Sprintf("%s/access_token?grant_type=ig_exchange_token&client_secret=%s&access_token=%s", baseURL, ClientSecret, accessToken)

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
