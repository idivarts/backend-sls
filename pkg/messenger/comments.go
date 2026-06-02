package messenger

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Graph comment operations for Facebook Pages (graph.facebook.com) using a Page
// access token. These also work for Instagram comment nodes belonging to an IG
// Business Account linked to the Page (Meta exposes those nodes on the Graph).
//
// For Instagram accounts connected via Instagram Login (graph.instagram.com),
// use the equivalent helpers in pkg/instagram.

// CommentItem is a single comment as returned by the Graph API.
type CommentItem struct {
	ID        string `json:"id"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp,omitempty"`
	From      struct {
		ID       string `json:"id"`
		Name     string `json:"name,omitempty"`
		Username string `json:"username,omitempty"`
	} `json:"from"`
}

type commentListResponse struct {
	Data []CommentItem `json:"data"`
}

type createIDResponse struct {
	ID string `json:"id"`
}

// CreateCommentReply posts a reply under a Facebook comment (or post).
// POST /{object-id}/comments?message=...
func CreateCommentReply(objectID, message, accessToken string) (string, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/comments", BaseURL, ApiVersion, objectID)
	form := url.Values{}
	form.Set("message", message)
	form.Set("access_token", accessToken)

	resp, err := http.PostForm(endpoint, form)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("CreateCommentReply: status %d: %s", resp.StatusCode, string(body))
	}
	var out createIDResponse
	_ = json.Unmarshal(body, &out)
	return out.ID, nil
}

// SetCommentHidden hides or unhides a Facebook comment.
// POST /{comment-id}?is_hidden=true|false
func SetCommentHidden(commentID string, hidden bool, accessToken string) error {
	endpoint := fmt.Sprintf("%s/%s/%s", BaseURL, ApiVersion, commentID)
	form := url.Values{}
	form.Set("is_hidden", fmt.Sprintf("%t", hidden))
	form.Set("access_token", accessToken)

	resp, err := http.PostForm(endpoint, form)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("SetCommentHidden: status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// DeleteObject deletes a Graph object (comment) by id.
// DELETE /{object-id}
func DeleteObject(objectID, accessToken string) error {
	endpoint := fmt.Sprintf("%s/%s/%s?access_token=%s", BaseURL, ApiVersion, objectID, url.QueryEscape(accessToken))
	req, err := http.NewRequest(http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("DeleteObject: status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// GetCommentReplies fetches replies under a comment.
// GET /{comment-id}/comments?fields=id,message,timestamp,from{id,name,username}
func GetCommentReplies(commentID, accessToken string) ([]CommentItem, error) {
	fields := "id,message,timestamp,from{id,name,username}"
	endpoint := fmt.Sprintf("%s/%s/%s/comments?fields=%s&access_token=%s",
		BaseURL, ApiVersion, commentID, url.QueryEscape(fields), url.QueryEscape(accessToken))
	resp, err := http.Get(endpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GetCommentReplies: status %d: %s", resp.StatusCode, string(body))
	}
	var out commentListResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// IsCommentID is a small heuristic: Graph comment ids contain an underscore
// ({page/post}_{comment}). Used only for logging/branching convenience.
func IsCommentID(id string) bool {
	return strings.Contains(id, "_")
}
