package messenger

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
)

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
}

func GetLongLivedAccessToken(accessToken string) (*TokenResponse, error) {
	url := fmt.Sprintf("%s/%s/oauth/access_token?grant_type=fb_exchange_token&client_id=%s&client_secret=%s&fb_exchange_token=%s", baseURL, apiVersion, ClientID, ClientSecret, accessToken)

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
