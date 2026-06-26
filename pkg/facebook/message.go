package facebook

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Request the media sub-fields explicitly — the default `attachments` expansion
// is not guaranteed to include file_url/mime_type, which we need to surface
// voice clips and file attachments alongside photos and videos.
const messageInfoFields = "id,created_time,from,to,message,attachments{id,mime_type,name,file_url,image_data,video_data}"

type Participants struct {
	Data []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`     // Facebook Messenger participant display name
		Username string `json:"username"` // Instagram-only handle (empty for Messenger)
	} `json:"data"`
}

type VideoData struct {
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	URL        string `json:"url"`
	PreviewURL string `json:"preview_url"`
}

type ImageData struct {
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	MaxWidth   int    `json:"max_width"`
	MaxHeight  int    `json:"max_height"`
	URL        string `json:"url"`
	PreviewURL string `json:"preview_url"`
}

type Cursors struct {
	Before string `json:"before"`
	After  string `json:"after"`
}

type Paging struct {
	Cursors Cursors `json:"cursors"`
	Next    string  `json:"next"`
}

type DataItem struct {
	ID        string     `json:"id,omitempty"`
	MimeType  string     `json:"mime_type,omitempty"`
	Name      string     `json:"name,omitempty"`
	FileURL   string     `json:"file_url,omitempty"`
	ImageData *ImageData `json:"image_data,omitempty"`
	VideoData *VideoData `json:"video_data,omitempty"`
}

type Attachments struct {
	Data   []DataItem `json:"data"`
	Paging Paging     `json:"paging"`
}

// MediaAttachment is a normalized view of a message's primary media attachment,
// independent of how Meta nested it (image_data / video_data / file_url).
type MediaAttachment struct {
	URL   string // direct media URL (may be a time-limited Meta CDN link)
	Type  string // image | video | audio | file
	Thumb string // optional preview/thumbnail (videos)
}

// FirstMedia returns the first usable media attachment on the message, or nil
// when it carries none. Meta folds shared posts, reels and story replies into
// image_data/video_data ("only the image or video URL for a share is returned"),
// so image_data + video_data + file_url together cover every media kind the
// Conversations API exposes — photos, videos, reels, shared posts, story
// replies, voice clips and file attachments.
func (m *Message) FirstMedia() *MediaAttachment {
	if m.Attachments == nil {
		return nil
	}
	for _, d := range m.Attachments.Data {
		switch {
		case d.VideoData != nil && d.VideoData.URL != "":
			return &MediaAttachment{URL: d.VideoData.URL, Type: "video", Thumb: d.VideoData.PreviewURL}
		case d.ImageData != nil && d.ImageData.URL != "":
			return &MediaAttachment{URL: d.ImageData.URL, Type: "image", Thumb: d.ImageData.PreviewURL}
		case d.FileURL != "":
			return &MediaAttachment{URL: d.FileURL, Type: mediaKindFromMime(d.MimeType)}
		}
	}
	return nil
}

// mediaKindFromMime maps a MIME type to a coarse attachment kind for file_url
// attachments (voice notes arrive as audio/*, etc.).
func mediaKindFromMime(mime string) string {
	switch {
	case strings.HasPrefix(mime, "audio"):
		return "audio"
	case strings.HasPrefix(mime, "video"):
		return "video"
	case strings.HasPrefix(mime, "image"):
		return "image"
	default:
		return "file"
	}
}

type Message struct {
	ID          string       `json:"id"`
	CreatedTime CustomTime   `json:"created_time"`
	To          Participants `json:"to"`
	From        struct {
		Username string `json:"username"`
		ID       string `json:"id"`
	} `json:"from"`
	Message     string       `json:"message"`
	Attachments *Attachments `json:"attachments,omitempty"`
}

func GetMessageInfo(messageID string, pageAccessToken string) (*Message, error) {
	// Set up the HTTP client
	client := http.Client{}

	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/%s/%s?fields=%s&access_token=%s", BaseURL, ApiVersion, messageID, messageInfoFields, pageAccessToken)

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
	data := Message{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, err
	}
	return &data, nil
}

// GetMessagesWithPagination fetches a page of messages for a conversation. The
// fetch retries with a shrinking page size on Meta's transient "Please reduce
// the amount of data you're asking for" 500s (the message attachment expansion
// is the heaviest part of a DM sync) — see GraphGetRetry.
func GetMessagesWithPagination(conversationID string, after string, limit int, pageAccessToken string) (*ConversationPaginatedMessageData, error) {
	body, err := GraphGetRetry(func(l int) string {
		return fmt.Sprintf("%s/%s/%s/messages?fields=%s&limit=%d&after=%s&access_token=%s", BaseURL, ApiVersion, conversationID, messageInfoFields, l, after, pageAccessToken)
	}, limit)
	if err != nil {
		return nil, err
	}

	data := ConversationPaginatedMessageData{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	return &data, nil
}
