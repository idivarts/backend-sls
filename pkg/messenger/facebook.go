package messenger

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

type FacebookProfile struct {
	ID           string `json:"id" firestore:"id"`
	Name         string `json:"name" firestore:"name"`
	About        string `json:"about" firestore:"about"`
	Category     string `json:"category" firestore:"category"`
	CategoryList []struct {
		ID   string `json:"id" firestore:"id"`
		Name string `json:"name" firestore:"name"`
	} `json:"category_list" firestore:"category_list"`
	Phone          string      `json:"phone" firestore:"phone"`
	Location       interface{} `json:"location" firestore:"location"` // You might want to define a struct for this
	Website        string      `json:"website" firestore:"website"`
	Emails         interface{} `json:"emails" firestore:"emails"` // You might want to define a struct for this
	FanCount       int         `json:"fan_count" firestore:"fan_count"`
	FollowersCount int         `json:"followers_count" firestore:"followers_count"`
	Picture        struct {
		Data struct {
			URL string `json:"url" firestore:"url"`
		} `json:"data" firestore:"data"`
	} `json:"picture" firestore:"picture"`
	Cover struct {
		Source string `json:"source" firestore:"source"`
		ID     string `json:"id" firestore:"id"`
	} `json:"cover" firestore:"cover"`
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
