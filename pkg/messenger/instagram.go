package messenger

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

type InstagramProfile struct {
	Name      string `json:"name"`
	Username  string `json:"username"`
	Biography string `json:"biography"`
	ID        string `json:"id"`
}

func GetInstagram(instagramId string, pageAccessToken string) (*InstagramProfile, error) {
	// Set up the HTTP client
	client := http.Client{}

	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/%s/%s?fields=name,username,biography&access_token=%s", baseURL, apiVersion, instagramId, pageAccessToken)

	// Make the API request
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("Error: Unexpected status code - " + resp.Status + "\n" + string(body))
	}

	// Print the response body
	fmt.Println(string(body))
	data := InstagramProfile{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, err
	}
	return &data, nil
}
