package youtube

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Channel represents a YouTube channel returned by the Data API v3.
type Channel struct {
	ID      string          `json:"id"`
	Snippet ChannelSnippet  `json:"snippet"`
	Stats   ChannelStats    `json:"statistics"`
}

type ChannelSnippet struct {
	Title       string          `json:"title"`
	Description string          `json:"description"`
	CustomURL   string          `json:"customUrl"` // e.g. @handle
	Country     string          `json:"country"`
	Thumbnails  ChannelThumbs   `json:"thumbnails"`
}

type ChannelThumbs struct {
	Default ThumbnailItem `json:"default"`
	Medium  ThumbnailItem `json:"medium"`
	High    ThumbnailItem `json:"high"`
}

type ThumbnailItem struct {
	URL string `json:"url"`
}

type ChannelStats struct {
	ViewCount             string `json:"viewCount"`
	SubscriberCount       string `json:"subscriberCount"`
	HiddenSubscriberCount bool   `json:"hiddenSubscriberCount"`
	VideoCount            string `json:"videoCount"`
}

type channelListResponse struct {
	Items []Channel `json:"items"`
}

// GetMyChannel returns the authenticated user's own YouTube channel.
// accessToken must have youtube.readonly scope.
func GetMyChannel(accessToken string) (*Channel, error) {
	url := APIURL + "/channels?part=snippet,statistics&mine=true"

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("youtube: failed to build channel request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("youtube: channel request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("youtube: channels endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var list channelListResponse
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("youtube: failed to parse channel response: %w", err)
	}
	if len(list.Items) == 0 {
		return nil, fmt.Errorf("youtube: no channel found for this account")
	}
	return &list.Items[0], nil
}
