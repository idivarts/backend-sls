package messenger

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

type FacebookProfile struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	About        string `json:"about"`
	Category     string `json:"category"`
	CategoryList []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"category_list"`
	Phone          string      `json:"phone"`
	Location       interface{} `json:"location"` // Think how to fix this
	Website        string      `json:"website"`
	Emails         interface{} `json:"emails"` // Think how to fix this
	FanCount       int         `json:"fan_count"`
	FollowersCount int         `json:"followers_count"`
	Picture        struct {
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
	} `json:"picture"`
	Cover struct {
		Source string `json:"source"`
		ID     string `json:"id"`
	} `json:"cover"`
}

func GetFacebook(pageAccessToken string) (*FacebookProfile, error) {
	// Set up the HTTP client
	client := http.Client{}

	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/%s/me?fields=id,name,about,category,category_list,location,phone,website,emails,fan_count,followers_count,picture{url},cover{source}&access_token=%s", baseURL, apiVersion, pageAccessToken)

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
	data := FacebookProfile{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, err
	}
	return &data, nil
}
