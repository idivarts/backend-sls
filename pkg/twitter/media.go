package twitter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// mediaUploadURL is the v2 chunked media upload endpoint.
const mediaUploadURL = APIURL + "/media/upload"

// maxChunkBytes is the maximum size of a single APPEND chunk (5MB).
const maxChunkBytes = 5 * 1024 * 1024

// downloadBytes fetches the bytes at srcURL and returns them along with the
// reported Content-Type (falling back to a sniffed type when the header is
// missing).
func downloadBytes(srcURL string) (data []byte, mimeType string, err error) {
	if strings.TrimSpace(srcURL) == "" {
		return nil, "", fmt.Errorf("twitter: empty media url")
	}
	resp, err := http.Get(srcURL)
	if err != nil {
		return nil, "", fmt.Errorf("twitter: failed to download media: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("twitter: media download returned %d for %s", resp.StatusCode, srcURL)
	}
	data, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("twitter: failed to read media body: %w", err)
	}
	mimeType = resp.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	return data, mimeType, nil
}

// mediaInitResponse is returned by the INIT command.
type mediaInitResponse struct {
	Data struct {
		ID            string `json:"id"`
		MediaIDString string `json:"media_id_string"`
	} `json:"data"`
	// Some responses surface these at the top level.
	MediaIDString string `json:"media_id_string"`
}

// mediaProcessingInfo describes asynchronous (video) processing state.
type mediaProcessingInfo struct {
	State          string `json:"state"` // pending, in_progress, succeeded, failed
	CheckAfterSecs int    `json:"check_after_secs"`
	ProgressPct    int    `json:"progress_percent"`
	Error          *struct {
		Code    int    `json:"code"`
		Name    string `json:"name"`
		Message string `json:"message"`
	} `json:"error"`
}

// mediaStatusResponse is returned by FINALIZE / STATUS commands.
type mediaStatusResponse struct {
	Data struct {
		ID             string               `json:"id"`
		MediaIDString  string               `json:"media_id_string"`
		ProcessingInfo *mediaProcessingInfo `json:"processing_info"`
	} `json:"data"`
	MediaIDString  string               `json:"media_id_string"`
	ProcessingInfo *mediaProcessingInfo `json:"processing_info"`
}

// pickMediaID returns the media id string from either the nested data object
// or the top-level field.
func (r mediaInitResponse) mediaID() string {
	if r.Data.MediaIDString != "" {
		return r.Data.MediaIDString
	}
	if r.Data.ID != "" {
		return r.Data.ID
	}
	return r.MediaIDString
}

func (r mediaStatusResponse) mediaID() string {
	if r.Data.MediaIDString != "" {
		return r.Data.MediaIDString
	}
	if r.Data.ID != "" {
		return r.Data.ID
	}
	return r.MediaIDString
}

func (r mediaStatusResponse) processingInfo() *mediaProcessingInfo {
	if r.Data.ProcessingInfo != nil {
		return r.Data.ProcessingInfo
	}
	return r.ProcessingInfo
}

// UploadMedia performs a v2 chunked media upload (INIT → APPEND → FINALIZE) and
// returns the resulting media_id_string. mediaCategory is one of
// "tweet_image", "tweet_gif", "tweet_video". For video/gif uploads it polls the
// FINALIZE processing_info until the media is ready (or fails).
func UploadMedia(accessToken string, data []byte, mediaCategory string) (mediaID string, err error) {
	if len(data) == 0 {
		return "", fmt.Errorf("twitter: cannot upload empty media")
	}
	mediaType := http.DetectContentType(data)

	// --- INIT ---
	mediaID, err = mediaInit(accessToken, len(data), mediaType, mediaCategory)
	if err != nil {
		return "", err
	}

	// --- APPEND ---
	if err = mediaAppend(accessToken, mediaID, data); err != nil {
		return "", err
	}

	// --- FINALIZE ---
	procInfo, err := mediaFinalize(accessToken, mediaID)
	if err != nil {
		return "", err
	}

	// --- Poll processing (video/gif) ---
	if procInfo != nil {
		if err = pollMediaProcessing(accessToken, mediaID, procInfo); err != nil {
			return "", err
		}
	}

	return mediaID, nil
}

func mediaInit(accessToken string, totalBytes int, mediaType, mediaCategory string) (string, error) {
	form := url.Values{}
	form.Set("command", "INIT")
	form.Set("total_bytes", strconv.Itoa(totalBytes))
	form.Set("media_type", mediaType)
	if mediaCategory != "" {
		form.Set("media_category", mediaCategory)
	}

	req, err := http.NewRequest(http.MethodPost, mediaUploadURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("twitter: failed to build media INIT request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("twitter: media INIT request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
		return "", fmt.Errorf("twitter: media INIT returned %d: %s", resp.StatusCode, string(body))
	}

	var r mediaInitResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return "", fmt.Errorf("twitter: failed to parse media INIT response: %w", err)
	}
	id := r.mediaID()
	if id == "" {
		return "", fmt.Errorf("twitter: media INIT returned no media_id: %s", string(body))
	}
	return id, nil
}

func mediaAppend(accessToken, mediaID string, data []byte) error {
	segmentIndex := 0
	for offset := 0; offset < len(data); offset += maxChunkBytes {
		end := offset + maxChunkBytes
		if end > len(data) {
			end = len(data)
		}
		chunk := data[offset:end]

		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		if err := writer.WriteField("command", "APPEND"); err != nil {
			return fmt.Errorf("twitter: failed to write APPEND field: %w", err)
		}
		if err := writer.WriteField("media_id", mediaID); err != nil {
			return fmt.Errorf("twitter: failed to write APPEND field: %w", err)
		}
		if err := writer.WriteField("segment_index", strconv.Itoa(segmentIndex)); err != nil {
			return fmt.Errorf("twitter: failed to write APPEND field: %w", err)
		}
		part, err := writer.CreateFormFile("media", "chunk")
		if err != nil {
			return fmt.Errorf("twitter: failed to create APPEND media part: %w", err)
		}
		if _, err := part.Write(chunk); err != nil {
			return fmt.Errorf("twitter: failed to write APPEND media chunk: %w", err)
		}
		if err := writer.Close(); err != nil {
			return fmt.Errorf("twitter: failed to close APPEND writer: %w", err)
		}

		req, err := http.NewRequest(http.MethodPost, mediaUploadURL, &buf)
		if err != nil {
			return fmt.Errorf("twitter: failed to build media APPEND request: %w", err)
		}
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+accessToken)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("twitter: media APPEND request failed: %w", err)
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusCreated {
			return fmt.Errorf("twitter: media APPEND returned %d: %s", resp.StatusCode, string(respBody))
		}
		segmentIndex++
	}
	return nil
}

// mediaFinalize sends FINALIZE and returns processing_info if the media needs
// asynchronous processing (video/gif), or nil if it is immediately ready.
func mediaFinalize(accessToken, mediaID string) (*mediaProcessingInfo, error) {
	form := url.Values{}
	form.Set("command", "FINALIZE")
	form.Set("media_id", mediaID)

	req, err := http.NewRequest(http.MethodPost, mediaUploadURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("twitter: failed to build media FINALIZE request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitter: media FINALIZE request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("twitter: media FINALIZE returned %d: %s", resp.StatusCode, string(body))
	}

	var r mediaStatusResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("twitter: failed to parse media FINALIZE response: %w", err)
	}
	return r.processingInfo(), nil
}

// pollMediaProcessing polls the media STATUS endpoint until the media reaches a
// terminal state (succeeded or failed).
func pollMediaProcessing(accessToken, mediaID string, info *mediaProcessingInfo) error {
	for {
		switch strings.ToLower(info.State) {
		case "succeeded":
			return nil
		case "failed":
			if info.Error != nil {
				return fmt.Errorf("twitter: media processing failed: %s (%d)", info.Error.Message, info.Error.Code)
			}
			return fmt.Errorf("twitter: media processing failed")
		}

		wait := info.CheckAfterSecs
		if wait <= 0 {
			wait = 1
		}
		time.Sleep(time.Duration(wait) * time.Second)

		next, err := mediaStatus(accessToken, mediaID)
		if err != nil {
			return err
		}
		if next == nil {
			// No processing_info means processing is complete.
			return nil
		}
		info = next
	}
}

func mediaStatus(accessToken, mediaID string) (*mediaProcessingInfo, error) {
	statusURL := fmt.Sprintf("%s?command=STATUS&media_id=%s", mediaUploadURL, url.QueryEscape(mediaID))

	req, err := http.NewRequest(http.MethodGet, statusURL, nil)
	if err != nil {
		return nil, fmt.Errorf("twitter: failed to build media STATUS request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitter: media STATUS request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitter: media STATUS returned %d: %s", resp.StatusCode, string(body))
	}

	var r mediaStatusResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("twitter: failed to parse media STATUS response: %w", err)
	}
	return r.processingInfo(), nil
}
