package instagram

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Instagram Content Publishing API (graph.instagram.com, Instagram Login).
// Two-step flow: create a media container, then publish it. Carousels create a
// child container per image first; videos/reels must finish processing before
// they can be published (see GetContainerStatus).

type idResponse struct {
	ID string `json:"id"`
}

type statusResponse struct {
	StatusCode string `json:"status_code"`
	ID         string `json:"id"`
}

// postForm issues a form-encoded POST to a Graph endpoint and returns the {id}.
func postForm(apiURL string, data url.Values) (string, error) {
	req, err := http.NewRequest("POST", apiURL, bytes.NewBufferString(data.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", errors.New("Error: Unexpected status code - " + resp.Status + "\n" + string(body))
	}

	out := idResponse{}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", err
	}
	return out.ID, nil
}

// CreateImageContainer creates a single-image feed container.
func CreateImageContainer(igUserID, imageURL, caption, accessToken string) (string, error) {
	apiURL := fmt.Sprintf("%s/%s/%s/media", BaseURL, ApiVersion, igUserID)
	data := url.Values{}
	data.Set("image_url", imageURL)
	if caption != "" {
		data.Set("caption", caption)
	}
	data.Set("access_token", accessToken)
	return postForm(apiURL, data)
}

// CreateReelContainer creates a video container. mediaType is "REELS" for reels
// or "STORIES" for a video story.
func CreateReelContainer(igUserID, videoURL, caption, mediaType, accessToken string) (string, error) {
	apiURL := fmt.Sprintf("%s/%s/%s/media", BaseURL, ApiVersion, igUserID)
	data := url.Values{}
	data.Set("media_type", mediaType)
	data.Set("video_url", videoURL)
	if caption != "" {
		data.Set("caption", caption)
	}
	data.Set("access_token", accessToken)
	return postForm(apiURL, data)
}

// CreateStoryImageContainer creates an image story container.
func CreateStoryImageContainer(igUserID, imageURL, accessToken string) (string, error) {
	apiURL := fmt.Sprintf("%s/%s/%s/media", BaseURL, ApiVersion, igUserID)
	data := url.Values{}
	data.Set("media_type", "STORIES")
	data.Set("image_url", imageURL)
	data.Set("access_token", accessToken)
	return postForm(apiURL, data)
}

// CreateCarouselItem creates a child image container for a carousel.
func CreateCarouselItem(igUserID, imageURL, accessToken string) (string, error) {
	apiURL := fmt.Sprintf("%s/%s/%s/media", BaseURL, ApiVersion, igUserID)
	data := url.Values{}
	data.Set("image_url", imageURL)
	data.Set("is_carousel_item", "true")
	data.Set("access_token", accessToken)
	return postForm(apiURL, data)
}

// CreateCarouselContainer wraps already-created child containers into a carousel.
func CreateCarouselContainer(igUserID string, childIDs []string, caption, accessToken string) (string, error) {
	apiURL := fmt.Sprintf("%s/%s/%s/media", BaseURL, ApiVersion, igUserID)
	data := url.Values{}
	data.Set("media_type", "CAROUSEL")
	data.Set("children", strings.Join(childIDs, ","))
	if caption != "" {
		data.Set("caption", caption)
	}
	data.Set("access_token", accessToken)
	return postForm(apiURL, data)
}

// PublishContainer publishes a created container and returns the media id.
func PublishContainer(igUserID, creationID, accessToken string) (string, error) {
	apiURL := fmt.Sprintf("%s/%s/%s/media_publish", BaseURL, ApiVersion, igUserID)
	data := url.Values{}
	data.Set("creation_id", creationID)
	data.Set("access_token", accessToken)
	return postForm(apiURL, data)
}

// GetContainerStatus returns a container's processing status — one of
// IN_PROGRESS, FINISHED, ERROR, EXPIRED, PUBLISHED. Used to gate video publishing.
func GetContainerStatus(containerID, accessToken string) (string, error) {
	apiURL := fmt.Sprintf("%s/%s/%s?fields=status_code&access_token=%s",
		BaseURL, ApiVersion, containerID, url.QueryEscape(accessToken))
	resp, err := http.Get(apiURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", errors.New("Error: Unexpected status code - " + resp.Status + "\n" + string(body))
	}
	st := statusResponse{}
	if err := json.Unmarshal(body, &st); err != nil {
		return "", err
	}
	return st.StatusCode, nil
}
