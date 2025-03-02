package messenger

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
)

type IFBPostsParams struct {
	Count int
}

type fbResponse struct {
	Data   []IPostData `json:"data"`
	Paging Paging      `json:"paging"`
}

type IPostData struct {
	Message      string     `json:"message"`
	FullPicture  string     `json:"full_picture"`
	ID           string     `json:"id"`
	PermalinkURL string     `json:"permalink_url"`
	CreatedTime  CustomTime `json:"created_time"`
}

func GetPosts(pageID, accessToken string, params IFBPostsParams) ([]IPostData, error) {
	// Set up the HTTP client
	client := http.Client{}

	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/%s/%s/posts", BaseURL, ApiVersion, pageID)

	// Create query parameters
	iParam := url.Values{}
	iParam.Set("fields", "message,full_picture,id,permalink_url,created_time")
	iParam.Set("access_token", accessToken)

	if params.Count == 0 {
		params.Count = 10
	}
	iParam.Set("limit", strconv.Itoa(params.Count))

	allParams := iParam.Encode()
	log.Println("All Params:", allParams)

	// Combine base URL and query parameters
	apiURL = fmt.Sprintf("%s?%s", apiURL, allParams)

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
	data := fbResponse{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, err
	}
	return data.Data, nil
}
