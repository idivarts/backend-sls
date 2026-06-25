package facebook

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type InstagramBriefProfile struct {
	Name      string `json:"name" firestore:"name"`
	Username  string `json:"username" firestore:"username"`
	Biography string `json:"biography" firestore:"biography"`
	ID        string `json:"id" firestore:"id"`
}

type InstagramProfile struct {
	InstagramBriefProfile
	ProfilePictureURL string             `json:"profile_picture_url" firestore:"profile_picture_url"`
	FollowersCount    int                `json:"followers_count" firestore:"followers_count"`
	FollowsCount      int                `json:"follows_count" firestore:"follows_count"`
	MediaCount        int                `json:"media_count" firestore:"media_count"`
	Website           string             `json:"website" firestore:"website"`
	ApproxMetrics     InstaApproxMetrics `json:"approxMetrics" firestore:"approxMetrics"`
}
type InstaApproxMetrics struct {
	Views        string `json:"views" firestore:"views"`
	Interactions string `json:"interactions" firestore:"interactions"`
	Followers    string `json:"followers" firestore:"followers"`
}

func GetInstagramInBrief(instagramId string, accessToken string) (*InstagramBriefProfile, error) {
	// Set up the HTTP client
	client := http.Client{}

	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/%s/%s?fields=name,username,biography&access_token=%s", BaseURL, ApiVersion, instagramId, accessToken)

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

// GetInstagramByUsername fetches a professional (business/creator) account's
// public profile (name, username, profile_picture_url, followers) by username via
// the Business Discovery API. selfIGID is the connected IG Business account id used
// as the query node; accessToken is its page token. Returns data only for
// professional target accounts (personal accounts are not discoverable this way).
// GET graph.facebook.com/{version}/{selfIGID}?fields=business_discovery.username(<username>){...}
func GetInstagramByUsername(selfIGID, username, accessToken string) (*InstagramProfile, error) {
	fields := fmt.Sprintf("business_discovery.username(%s){id,name,username,profile_picture_url,followers_count}", username)
	apiURL := fmt.Sprintf("%s/%s/%s?fields=%s&access_token=%s",
		BaseURL, ApiVersion, selfIGID, url.QueryEscape(fields), url.QueryEscape(accessToken))

	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GetInstagramByUsername: status %d: %s", resp.StatusCode, string(body))
	}
	var out struct {
		BusinessDiscovery InstagramProfile `json:"business_discovery"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return &out.BusinessDiscovery, nil
}

func GetInstagram(instagramId string, accessToken string) (*InstagramProfile, error) {
	// Set up the HTTP client
	client := http.Client{}

	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/%s/%s?fields=id,name,username,profile_picture_url,biography,followers_count,follows_count,media_count,website&access_token=%s", BaseURL, ApiVersion, instagramId, accessToken)

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
