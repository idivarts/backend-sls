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
	Phone                    string      `json:"phone" firestore:"phone"`
	Location                 interface{} `json:"location" firestore:"location"` // You might want to define a struct for this
	Website                  string      `json:"website" firestore:"website"`
	Emails                   interface{} `json:"emails" firestore:"emails"` // You might want to define a struct for this
	Email                    *string     `json:"email,omitempty" firestore:"email,omitempty"`
	FanCount                 int         `json:"fan_count" firestore:"fan_count"`
	FollowersCount           int         `json:"followers_count" firestore:"followers_count"`
	InstagramBusinessAccount *struct {
		ID string `json:"id"`
	} `json:"instagram_business_account,omitempty"`
	Picture struct {
		Data struct {
			URL string `json:"url" firestore:"url"`
		} `json:"data" firestore:"data"`
	} `json:"picture" firestore:"picture"`
	Cover struct {
		Source string `json:"source" firestore:"source"`
		ID     string `json:"id" firestore:"id"`
	} `json:"cover" firestore:"cover"`
}

type FacebookProfileAccounts struct {
	Accounts struct {
		Data []FacebookProfile `json:"data"`
	} `json:"accounts"`
}

func GetMyFacebook(pageId, accessToken string) (*FacebookProfile, *FacebookProfileAccounts, error) {
	// Set the API endpoint
	// apiURL := fmt.Sprintf("%s/%s/%s?fields=id,name,about,location,website,email,picture{url},accounts{id,name,about,category,category_list,location,website,emails,fan_count,followers_count,picture{url},cover{source},instagram_business_account}&access_token=%s", BaseURL, ApiVersion, pageId, accessToken)
	apiURL := fmt.Sprintf("%s/%s/%s?fields=id,name,email,picture{url},accounts{id,name,followers_count,picture{url},instagram_business_account}&access_token=%s", BaseURL, ApiVersion, pageId, accessToken)
	return getFacebook(apiURL)
}

func GetFacebookPage(pageId, accessToken string) (*FacebookProfile, error) {
	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/%s/%s?fields=id,name,about,category,category_list,location,phone,website,emails,fan_count,followers_count,picture{url},cover{source}&access_token=%s", BaseURL, ApiVersion, pageId, accessToken)
	fb, _, err := getFacebook(apiURL)
	return fb, err
}

func getFacebook(apiURL string) (*FacebookProfile, *FacebookProfileAccounts, error) {
	// Set up the HTTP client
	client := http.Client{}

	// Make the API request
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, nil, errors.New("Error: Unexpected status code - " + resp.Status + "\n" + string(body))
	}

	// Print the response body
	fmt.Println(string(body))
	data := FacebookProfile{}

	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, nil, err
	}

	rawMap := FacebookProfileAccounts{}
	err = json.Unmarshal(body, &rawMap)
	if err != nil {
		return nil, nil, err
	}

	if len(rawMap.Accounts.Data) > 0 {
		return &data, &rawMap, nil
	} else {
		return &data, nil, nil
	}

}
