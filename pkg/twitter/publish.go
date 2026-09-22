package twitter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// createTweetRequest is the JSON body for POST /2/tweets.
type createTweetRequest struct {
	Text  string            `json:"text"`
	Media *createTweetMedia `json:"media,omitempty"`
	Reply *createTweetReply `json:"reply,omitempty"`
}

type createTweetMedia struct {
	MediaIDs []string `json:"media_ids"`
}

type createTweetReply struct {
	InReplyToTweetID string `json:"in_reply_to_tweet_id"`
}

type createTweetResponse struct {
	Data struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	} `json:"data"`
}

// CreateTweet posts a tweet (optionally with attached media and/or as a reply)
// and returns the new tweet's id.
func CreateTweet(accessToken, text string, mediaIDs []string, inReplyToTweetID string) (tweetID string, err error) {
	reqBody := createTweetRequest{Text: text}
	if len(mediaIDs) > 0 {
		reqBody.Media = &createTweetMedia{MediaIDs: mediaIDs}
	}
	if strings.TrimSpace(inReplyToTweetID) != "" {
		reqBody.Reply = &createTweetReply{InReplyToTweetID: inReplyToTweetID}
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("twitter: failed to marshal tweet body: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, APIURL+"/tweets", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("twitter: failed to build create tweet request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("twitter: create tweet request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("twitter: create tweet returned %d: %s", resp.StatusCode, string(body))
	}

	var r createTweetResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return "", fmt.Errorf("twitter: failed to parse create tweet response: %w", err)
	}
	if r.Data.ID == "" {
		return "", fmt.Errorf("twitter: create tweet returned no id: %s", string(body))
	}
	return r.Data.ID, nil
}

// PublishTweet uploads any attached media (a single video, or up to 4 images)
// and then creates the tweet. videoURL takes precedence over imageURLs.
func PublishTweet(accessToken, text string, imageURLs []string, videoURL string) (tweetID string, err error) {
	var mediaIDs []string

	if strings.TrimSpace(videoURL) != "" {
		data, _, err := downloadBytes(videoURL)
		if err != nil {
			return "", err
		}
		id, err := UploadMedia(accessToken, data, "tweet_video")
		if err != nil {
			return "", err
		}
		mediaIDs = append(mediaIDs, id)
	} else {
		for _, imageURL := range imageURLs {
			if strings.TrimSpace(imageURL) == "" {
				continue
			}
			if len(mediaIDs) >= 4 {
				break
			}
			data, _, err := downloadBytes(imageURL)
			if err != nil {
				return "", err
			}
			id, err := UploadMedia(accessToken, data, "tweet_image")
			if err != nil {
				return "", err
			}
			mediaIDs = append(mediaIDs, id)
		}
	}

	return CreateTweet(accessToken, strings.TrimRight(text, " "), mediaIDs, "")
}
