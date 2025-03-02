package instagram

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"

	"github.com/idivarts/backend-sls/pkg/messenger"
)

type InstagramMedia struct {
	Caption       string               `json:"caption"`
	MediaType     string               `json:"media_type"`
	MediaURL      string               `json:"media_url"`
	ThumbnailURL  string               `json:"thumbnail_url,omitempty"` // Optional, as not all items might have this
	Permalink     string               `json:"permalink"`
	Timestamp     messenger.CustomTime `json:"timestamp"`
	CommentsCount int                  `json:"comments_count"`
	LikeCount     int                  `json:"like_count"`
	ID            string               `json:"id"`
}

type instaResponse struct {
	Data []InstagramMedia `json:"data"`
}

type IGetMediaParams struct {
	GraphType int
	PageID    string
	Count     int
}

func GetMedia(pageID, accessToken string, params IGetMediaParams) ([]InstagramMedia, error) {
	// Set up the HTTP client
	client := http.Client{}

	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/%s/%s/media", BaseURL, ApiVersion, pageID)
	if params.GraphType == 0 {
		if params.PageID == "" {
			return nil, fmt.Errorf("pageID is required for instagram - %s", params.PageID)
		}
		apiURL = fmt.Sprintf("%s/%s/%s/media", messenger.BaseURL, messenger.ApiVersion, params.PageID)
	}
	// Create query parameters
	iParam := url.Values{}
	iParam.Set("fields", "caption,media_type,media_url,thumbnail_url,cover_url,permalink,timestamp,comments_count,like_count")
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
	data := instaResponse{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, err
	}
	return data.Data, nil
}
