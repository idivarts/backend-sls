package facebook

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
)

// Facebook Page publishing (graph.facebook.com). Uses a Page access token.

type FBPublishResponse struct {
	ID     string `json:"id"`
	PostID string `json:"post_id"`
}

// PublishPagePhoto posts a photo to a Facebook Page.
//
// The image bytes are uploaded directly as multipart form-data (the `source`
// field) rather than handing Facebook the image URL to scrape. Letting Facebook
// fetch the URL itself intermittently fails with error 324 / subcode 2069019
// ("Missing or invalid image file", error_user_title "Image Required",
// is_transient: true) even for perfectly valid, publicly reachable images —
// Facebook's scraper sometimes can't pull our CDN URL in time. Uploading the
// bytes removes that dependency and is the reliable path.
//
// If we cannot download the image server-side, we fall back to the legacy
// URL-scrape so a transient outbound-fetch issue doesn't block publishing.
func PublishPagePhoto(pageID, imageURL, caption, pageAccessToken string) (*FBPublishResponse, error) {
	apiURL := fmt.Sprintf("%s/%s/%s/photos", BaseURL, ApiVersion, pageID)

	imgBytes, contentType, err := downloadImage(imageURL)
	if err != nil {
		data := url.Values{}
		data.Set("url", imageURL)
		if caption != "" {
			data.Set("caption", caption)
		}
		data.Set("access_token", pageAccessToken)
		return fbPost(apiURL, data)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("source", "image"+extForContentType(contentType))
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(imgBytes); err != nil {
		return nil, err
	}
	if caption != "" {
		_ = writer.WriteField("caption", caption)
	}
	_ = writer.WriteField("access_token", pageAccessToken)
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return fbPostMultipart(apiURL, writer.FormDataContentType(), &body)
}

// downloadImage fetches the image bytes (and its content type) so they can be
// uploaded straight to Facebook instead of relying on Facebook's URL scraper.
func downloadImage(imageURL string) ([]byte, string, error) {
	if strings.TrimSpace(imageURL) == "" {
		return nil, "", errors.New("empty image url")
	}
	resp, err := http.Get(imageURL)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("download image: unexpected status %s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	return data, contentType, nil
}

func extForContentType(ct string) string {
	switch {
	case strings.Contains(ct, "png"):
		return ".png"
	case strings.Contains(ct, "webp"):
		return ".webp"
	case strings.Contains(ct, "gif"):
		return ".gif"
	default:
		return ".jpg"
	}
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
	return fbDo(req)
}

// fbPostMultipart POSTs a multipart/form-data body (used to upload image bytes
// directly to the Graph API).
func fbPostMultipart(apiURL, contentType string, body *bytes.Buffer) (*FBPublishResponse, error) {
	req, err := http.NewRequest("POST", apiURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return fbDo(req)
}

func fbDo(req *http.Request) (*FBPublishResponse, error) {
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
