package youtube

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// UploadOptions describes a video to be published via videos.insert.
// PrivacyStatus must be one of "public", "private", or "unlisted".
// PublishAt is an RFC3339 timestamp; when set, PrivacyStatus MUST be "private"
// and YouTube auto-publishes the video at that time. CategoryID defaults to
// "22" (People & Blogs) when empty.
type UploadOptions struct {
	Title         string
	Description   string
	Tags          []string
	CategoryID    string
	PrivacyStatus string
	PublishAt     string
	VideoURL      string
	MadeForKids   bool
}

// videoSnippet / videoStatus mirror the videos.insert request body.
type videoSnippet struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags,omitempty"`
	CategoryID  string   `json:"categoryId"`
}

type videoStatus struct {
	PrivacyStatus           string `json:"privacyStatus"`
	PublishAt               string `json:"publishAt,omitempty"`
	SelfDeclaredMadeForKids bool   `json:"selfDeclaredMadeForKids"`
}

type videoInsertRequest struct {
	Snippet videoSnippet `json:"snippet"`
	Status  videoStatus  `json:"status"`
}

type videoResource struct {
	ID string `json:"id"`
}

// downloadBytes fetches the bytes (and content type) at the given URL.
// Returns an error if the URL is empty or the response is not 200.
func downloadBytes(fileURL string) ([]byte, string, error) {
	if strings.TrimSpace(fileURL) == "" {
		return nil, "", fmt.Errorf("youtube: empty download url")
	}
	resp, err := http.Get(fileURL)
	if err != nil {
		return nil, "", fmt.Errorf("youtube: download request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("youtube: download returned %d for %s", resp.StatusCode, fileURL)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("youtube: failed to read download body: %w", err)
	}
	if len(data) == 0 {
		return nil, "", fmt.Errorf("youtube: downloaded empty body from %s", fileURL)
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	return data, contentType, nil
}

// PublishVideo uploads a video to YouTube via the resumable upload protocol and
// returns the new video's ID. accessToken must have the youtube.upload scope.
func PublishVideo(accessToken string, opt UploadOptions) (string, error) {
	// 1. Download the video bytes.
	videoBytes, _, err := downloadBytes(opt.VideoURL)
	if err != nil {
		return "", err
	}

	categoryID := opt.CategoryID
	if categoryID == "" {
		categoryID = "22"
	}

	reqBody := videoInsertRequest{
		Snippet: videoSnippet{
			Title:       opt.Title,
			Description: opt.Description,
			Tags:        opt.Tags,
			CategoryID:  categoryID,
		},
		Status: videoStatus{
			PrivacyStatus:           opt.PrivacyStatus,
			PublishAt:               opt.PublishAt,
			SelfDeclaredMadeForKids: opt.MadeForKids,
		},
	}
	metaJSON, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("youtube: failed to marshal upload metadata: %w", err)
	}

	// 2. Initiate the resumable session.
	initURL := UploadURL + "?uploadType=resumable&part=snippet,status"
	initReq, err := http.NewRequest(http.MethodPost, initURL, bytes.NewReader(metaJSON))
	if err != nil {
		return "", fmt.Errorf("youtube: failed to build resumable init request: %w", err)
	}
	initReq.Header.Set("Authorization", "Bearer "+accessToken)
	initReq.Header.Set("Content-Type", "application/json")
	initReq.Header.Set("X-Upload-Content-Type", "video/*")

	initResp, err := http.DefaultClient.Do(initReq)
	if err != nil {
		return "", fmt.Errorf("youtube: resumable init request failed: %w", err)
	}
	defer initResp.Body.Close()

	initBody, _ := io.ReadAll(initResp.Body)
	if initResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("youtube: resumable init returned %d: %s", initResp.StatusCode, string(initBody))
	}

	sessionURL := initResp.Header.Get("Location")
	if sessionURL == "" {
		return "", fmt.Errorf("youtube: resumable init missing Location header")
	}

	// 3. Upload the video bytes to the session URL.
	putReq, err := http.NewRequest(http.MethodPut, sessionURL, bytes.NewReader(videoBytes))
	if err != nil {
		return "", fmt.Errorf("youtube: failed to build upload request: %w", err)
	}
	putReq.Header.Set("Authorization", "Bearer "+accessToken)
	putReq.Header.Set("Content-Type", "video/*")

	putResp, err := http.DefaultClient.Do(putReq)
	if err != nil {
		return "", fmt.Errorf("youtube: video upload request failed: %w", err)
	}
	defer putResp.Body.Close()

	putBody, _ := io.ReadAll(putResp.Body)
	if putResp.StatusCode != http.StatusOK && putResp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("youtube: video upload returned %d: %s", putResp.StatusCode, string(putBody))
	}

	var video videoResource
	if err := json.Unmarshal(putBody, &video); err != nil {
		return "", fmt.Errorf("youtube: failed to parse uploaded video response: %w", err)
	}
	if video.ID == "" {
		return "", fmt.Errorf("youtube: uploaded video response missing id: %s", string(putBody))
	}
	return video.ID, nil
}

// SetThumbnail sets a custom thumbnail for a video. It downloads the image at
// imageURL and uploads it via thumbnails.set. Best-effort: callers may ignore
// the error. accessToken must have the youtube.force-ssl (or youtube.upload) scope.
func SetThumbnail(accessToken, videoID, imageURL string) error {
	imageBytes, contentType, err := downloadBytes(imageURL)
	if err != nil {
		return err
	}
	if contentType == "" {
		contentType = "image/jpeg"
	}

	uploadURL := fmt.Sprintf("https://www.googleapis.com/upload/youtube/v3/thumbnails/set?videoId=%s&uploadType=media", videoID)
	req, err := http.NewRequest(http.MethodPost, uploadURL, bytes.NewReader(imageBytes))
	if err != nil {
		return fmt.Errorf("youtube: failed to build thumbnail request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", contentType)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("youtube: thumbnail request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("youtube: thumbnails.set returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
