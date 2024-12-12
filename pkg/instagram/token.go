package instagram

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
)

type CodeResponseData struct {
	Data []CodeResponse `json:"data"`
}
type CodeResponse struct {
	AccessToken string   `json:"access_token"`
	UserID      string   `json:"user_id"`
	Permissions []string `json:"permissions"`
}

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   string `json:"expires_in"`
}

func GetAccessTokenFromCode(code string) (*CodeResponse, error) {
	url := fmt.Sprintf("%s/oauth/access_token?grant_type=authorization_code&client_id=%s&client_secret=%s&code=%s", apiURL, ClientID, ClientSecret, code)

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
	token := &CodeResponseData{}
	err = json.Unmarshal(b, token)
	if err != nil {
		return nil, err
	}
	if token.Data == nil || len(token.Data) == 0 {
		return nil, errors.New("invalid code")
	}
	return &token.Data[0], nil
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
