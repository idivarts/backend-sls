package twitter

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Tweet represents a tweet from the v2 API along with hydrated author details
// and flattened public metrics.
type Tweet struct {
	ID             string    `json:"id"`
	Text           string    `json:"text"`
	AuthorID       string    `json:"author_id"`
	ConversationID string    `json:"conversation_id"`
	CreatedAt      time.Time `json:"created_at"`

	// Non-API fields, hydrated from includes.users.
	AuthorUsername string
	AuthorName     string
	AuthorAvatar   string

	// Flattened public_metrics.
	Likes       int64
	Replies     int64
	Retweets    int64
	Quotes      int64
	Impressions int64
}

// tweetPayload is the raw API shape used to decode tweet objects.
type tweetPayload struct {
	ID             string `json:"id"`
	Text           string `json:"text"`
	AuthorID       string `json:"author_id"`
	ConversationID string `json:"conversation_id"`
	CreatedAt      string `json:"created_at"`
	PublicMetrics  struct {
		LikeCount       int64 `json:"like_count"`
		ReplyCount      int64 `json:"reply_count"`
		RetweetCount    int64 `json:"retweet_count"`
		QuoteCount      int64 `json:"quote_count"`
		ImpressionCount int64 `json:"impression_count"`
	} `json:"public_metrics"`
	NonPublicMetrics struct {
		ImpressionCount int64 `json:"impression_count"`
	} `json:"non_public_metrics"`
	OrganicMetrics struct {
		ImpressionCount int64 `json:"impression_count"`
	} `json:"organic_metrics"`
}

// includesUser is the hydrated author object from expansions.
type includesUser struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Username        string `json:"username"`
	ProfileImageURL string `json:"profile_image_url"`
}

// tweetsListResponse covers timeline / search responses.
type tweetsListResponse struct {
	Data     []tweetPayload `json:"data"`
	Includes struct {
		Users []includesUser `json:"users"`
	} `json:"includes"`
}

func (p tweetPayload) toTweet() Tweet {
	t := Tweet{
		ID:             p.ID,
		Text:           p.Text,
		AuthorID:       p.AuthorID,
		ConversationID: p.ConversationID,
		Likes:          p.PublicMetrics.LikeCount,
		Replies:        p.PublicMetrics.ReplyCount,
		Retweets:       p.PublicMetrics.RetweetCount,
		Quotes:         p.PublicMetrics.QuoteCount,
		Impressions:    p.PublicMetrics.ImpressionCount,
	}
	if p.CreatedAt != "" {
		if parsed, err := time.Parse(time.RFC3339, p.CreatedAt); err == nil {
			t.CreatedAt = parsed
		}
	}
	return t
}

// hydrateAuthors fills author username/name/avatar on each tweet using the
// includes.users list matched by author_id.
func hydrateAuthors(tweets []Tweet, users []includesUser) {
	if len(users) == 0 {
		return
	}
	byID := make(map[string]includesUser, len(users))
	for _, u := range users {
		byID[u.ID] = u
	}
	for i := range tweets {
		if u, ok := byID[tweets[i].AuthorID]; ok {
			tweets[i].AuthorUsername = u.Username
			tweets[i].AuthorName = u.Name
			tweets[i].AuthorAvatar = u.ProfileImageURL
		}
	}
}

// getTweets performs a GET against the given URL and decodes a tweet list
// (with author hydration).
func getTweets(accessToken, requestURL, errContext string) ([]Tweet, error) {
	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("twitter: failed to build %s request: %w", errContext, err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitter: %s request failed: %w", errContext, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitter: %s returned %d: %s", errContext, resp.StatusCode, string(body))
	}

	var r tweetsListResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("twitter: failed to parse %s response: %w", errContext, err)
	}

	tweets := make([]Tweet, 0, len(r.Data))
	for _, p := range r.Data {
		tweets = append(tweets, p.toTweet())
	}
	hydrateAuthors(tweets, r.Includes.Users)
	return tweets, nil
}

// GetUserTweets returns a user's recent original tweets (excluding replies and
// retweets).
func GetUserTweets(accessToken, userID string, maxResults int) ([]Tweet, error) {
	q := url.Values{}
	q.Set("max_results", strconv.Itoa(maxResults))
	q.Set("tweet.fields", "created_at,public_metrics,conversation_id")
	q.Set("exclude", "replies,retweets")
	requestURL := fmt.Sprintf("%s/users/%s/tweets?%s", APIURL, userID, q.Encode())
	return getTweets(accessToken, requestURL, "user tweets")
}

// GetReplies returns recent replies in a conversation thread.
func GetReplies(accessToken, conversationID string) ([]Tweet, error) {
	q := url.Values{}
	q.Set("query", "conversation_id:"+conversationID)
	q.Set("tweet.fields", "created_at,public_metrics,author_id")
	q.Set("expansions", "author_id")
	q.Set("user.fields", "name,username,profile_image_url")
	requestURL := fmt.Sprintf("%s/tweets/search/recent?%s", APIURL, q.Encode())
	return getTweets(accessToken, requestURL, "replies search")
}

// GetMentions returns recent tweets mentioning the user.
func GetMentions(accessToken, userID string, maxResults int) ([]Tweet, error) {
	q := url.Values{}
	q.Set("max_results", strconv.Itoa(maxResults))
	q.Set("tweet.fields", "created_at,public_metrics,author_id")
	q.Set("expansions", "author_id")
	q.Set("user.fields", "name,username,profile_image_url")
	requestURL := fmt.Sprintf("%s/users/%s/mentions?%s", APIURL, userID, q.Encode())
	return getTweets(accessToken, requestURL, "mentions")
}

// ReplyToTweet posts a text reply to the given tweet.
func ReplyToTweet(accessToken, inReplyToTweetID, text string) (tweetID string, err error) {
	return CreateTweet(accessToken, text, nil, inReplyToTweetID)
}

// DeleteTweet deletes a tweet by id.
func DeleteTweet(accessToken, tweetID string) error {
	requestURL := fmt.Sprintf("%s/tweets/%s", APIURL, tweetID)

	req, err := http.NewRequest(http.MethodDelete, requestURL, nil)
	if err != nil {
		return fmt.Errorf("twitter: failed to build delete tweet request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("twitter: delete tweet request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("twitter: delete tweet returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
