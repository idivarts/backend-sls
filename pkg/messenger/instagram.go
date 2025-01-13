package messenger

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

type InstagramBriefProfile struct {
	Name      string `json:"name" firestore:"name"`
	Username  string `json:"username" firestore:"username"`
	Biography string `json:"biography" firestore:"biography"`
	ID        string `json:"id" firestore:"id"`
}

type InstagramProfile struct {
	InstagramBriefProfile
	ProfilePictureURL string `json:"profile_picture_url" firestore:"profile_picture_url"`
	FollowersCount    int    `json:"followers_count" firestore:"followers_count"`
	FollowsCount      int    `json:"follows_count" firestore:"follows_count"`
	MediaCount        int    `json:"media_count" firestore:"media_count"`
	Website           string `json:"website" firestore:"website"`
}

func GetInstagramInBrief(instagramId string, pageAccessToken string) (*InstagramBriefProfile, error) {
	// Set up the HTTP client
	client := http.Client{}

	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/%s/%s?fields=name,username,biography&access_token=%s", BaseURL, ApiVersion, instagramId, pageAccessToken)

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
	data := InstagramBriefProfile{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, err
	}
	return &data, nil
}

func GetInstagram(instagramId string, pageAccessToken string) (*InstagramProfile, error) {
	// Set up the HTTP client
	client := http.Client{}

	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/%s/%s?fields=id,name,username,profile_picture_url,biography,followers_count,follows_count,media_count,website&access_token=%s", BaseURL, ApiVersion, instagramId, pageAccessToken)

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
