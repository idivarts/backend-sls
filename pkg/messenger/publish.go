package messenger

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// Facebook Page publishing (graph.facebook.com). Uses a Page access token.

type FBPublishResponse struct {
	ID     string `json:"id"`
	PostID string `json:"post_id"`
}

// PublishPagePhoto posts a photo (by URL) to a Facebook Page.
func PublishPagePhoto(pageID, imageURL, caption, pageAccessToken string) (*FBPublishResponse, error) {
	apiURL := fmt.Sprintf("%s/%s/%s/photos", BaseURL, ApiVersion, pageID)
	data := url.Values{}
	data.Set("url", imageURL)
	if caption != "" {
		data.Set("caption", caption)
	}
	data.Set("access_token", pageAccessToken)
	return fbPost(apiURL, data)
}

// PublishPageFeed posts a text (optionally with a link) status to a Page feed.
func PublishPageFeed(pageID, message, link, pageAccessToken string) (*FBPublishResponse, error) {
	apiURL := fmt.Sprintf("%s/%s/%s/feed", BaseURL, ApiVersion, pageID)
	data := url.Values{}
	if message != "" {
		data.Set("message", message)
	}
	if link != "" {
		data.Set("link", link)
	}
	data.Set("access_token", pageAccessToken)
	return fbPost(apiURL, data)
}

func fbPost(apiURL string, data url.Values) (*FBPublishResponse, error) {
	req, err := http.NewRequest("POST", apiURL, bytes.NewBufferString(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, errors.New("Error: Unexpected status code - " + resp.Status + "\n" + string(body))
	}
	out := &FBPublishResponse{}
	if err := json.Unmarshal(body, out); err != nil {
		return nil, err
	}
	return out, nil
}
