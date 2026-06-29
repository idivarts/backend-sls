package linkedin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// OrgPost is a single post authored by an organization (Company Page), as
// returned by the versioned Posts API when queried by author.
type OrgPost struct {
	URN          string `json:"urn"`
	Text         string `json:"text"`
	Permalink    string `json:"permalink"`
	CreatedAt    int64  `json:"createdAt"` // epoch ms
	ThumbnailURL string `json:"thumbnailUrl"`
	CommentCount int64  `json:"commentCount"`
	LikeCount    int64  `json:"likeCount"`
}

// Comment is a single comment on a post (or a reply to a comment), as returned
// by the socialActions/{urn}/comments API.
type Comment struct {
	URN         string `json:"urn"`
	Text        string `json:"text"`
	ActorURN    string `json:"actorUrn"`
	ActorName   string `json:"actorName"`
	ActorAvatar string `json:"actorAvatar"`
	CreatedAt   int64  `json:"createdAt"` // epoch ms
}

// ListOrgPosts returns the most recent posts authored by the organization via
// the versioned Posts API (q=author). count caps the number returned.
// CommentCount/LikeCount are left at 0 here — fill them lazily via social
// metadata at the call site if needed.
func ListOrgPosts(accessToken, orgURN string, count int) ([]OrgPost, error) {
	if count <= 0 {
		count = 10
	}
	u := fmt.Sprintf(
		"%s/posts?q=author&author=%s&count=%d&sortBy=LAST_MODIFIED",
		RestBaseURL, url.QueryEscape(orgURN), count,
	)
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("linkedin: build posts request: %w", err)
	}
	restHeaders(req, accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("linkedin: posts request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("linkedin: posts (q=author) returned %d: %s", resp.StatusCode, string(body))
	}

	var parsed struct {
		Elements []struct {
			ID         string `json:"id"`         // post URN, e.g. urn:li:share:... / urn:li:ugcPost:...
			Commentary string `json:"commentary"` // post body text
			CreatedAt  int64  `json:"createdAt"`  // epoch ms
		} `json:"elements"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("linkedin: parse posts: %w", err)
	}

	posts := make([]OrgPost, 0, len(parsed.Elements))
	for _, e := range parsed.Elements {
		posts = append(posts, OrgPost{
			URN:       e.ID,
			Text:      e.Commentary,
			Permalink: "https://www.linkedin.com/feed/update/" + e.ID,
			CreatedAt: e.CreatedAt,
			// CommentCount / LikeCount left at 0 — filled lazily by caller.
		})
	}
	return posts, nil
}

// GetComments returns the comments on an object (post URN or comment URN) via
// the socialActions comments API. ActorName/ActorAvatar are best-effort and
// left empty unless present in the response (no extra per-comment calls).
func GetComments(accessToken, objectURN string) ([]Comment, error) {
	u := fmt.Sprintf("%s/socialActions/%s/comments", RestBaseURL, url.PathEscape(objectURN))
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("linkedin: build comments request: %w", err)
	}
	restHeaders(req, accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("linkedin: comments request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("linkedin: socialActions comments returned %d: %s", resp.StatusCode, string(body))
	}

	var parsed struct {
		Elements []struct {
			ID         string `json:"id"`
			CommentURN string `json:"commentUrn"`
			Actor      string `json:"actor"`
			Message    struct {
				Text string `json:"text"`
			} `json:"message"`
			Created struct {
				Time int64 `json:"time"`
			} `json:"created"`
		} `json:"elements"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("linkedin: parse comments: %w", err)
	}

	comments := make([]Comment, 0, len(parsed.Elements))
	for _, e := range parsed.Elements {
		urn := e.ID
		if urn == "" {
			urn = e.CommentURN
		}
		comments = append(comments, Comment{
			URN:       urn,
			Text:      e.Message.Text,
			ActorURN:  e.Actor,
			CreatedAt: e.Created.Time,
			// ActorName / ActorAvatar best-effort: left empty (no per-comment calls).
		})
	}
	return comments, nil
}

// CreateCommentReply posts a comment on objectURN (a post or comment URN) as
// actorURN. When parentCommentURN is non-empty the new comment is threaded as a
// reply to it. Returns the created comment's URN (from the response body id or
// the x-restli-id header).
func CreateCommentReply(accessToken, actorURN, objectURN, parentCommentURN, text string) (commentURN string, err error) {
	payload := map[string]interface{}{
		"actor": actorURN,
		"message": map[string]interface{}{
			"text": text,
		},
	}
	if parentCommentURN != "" {
		payload["parentComment"] = parentCommentURN
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("linkedin: marshal comment: %w", err)
	}

	u := fmt.Sprintf("%s/socialActions/%s/comments", RestBaseURL, url.PathEscape(objectURN))
	req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("linkedin: build create-comment request: %w", err)
	}
	restHeaders(req, accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("linkedin: create-comment request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("linkedin: create-comment returned %d: %s", resp.StatusCode, string(respBody))
	}

	// Prefer the URN from the response body; fall back to the header.
	var parsed struct {
		ID         string `json:"id"`
		CommentURN string `json:"commentUrn"`
	}
	_ = json.Unmarshal(respBody, &parsed)
	if parsed.ID != "" {
		return parsed.ID, nil
	}
	if parsed.CommentURN != "" {
		return parsed.CommentURN, nil
	}
	if h := resp.Header.Get("x-restli-id"); h != "" {
		return h, nil
	}
	return resp.Header.Get("x-linkedin-id"), nil
}

// DeleteComment deletes a comment as actorURN.
//
// NOTE: The exact CMA delete-comment path must be verified against current
// Community Management API docs. LinkedIn's documented form is
// DELETE /rest/socialActions/{objectUrn}/comments/{commentId}?actor=...,
// which requires both the parent object URN and the bare comment id; we only
// have the comment URN here, so we use the standalone comments resource
// (DELETE /rest/comments/{commentUrn}?actor=...). Verify before relying on it.
func DeleteComment(accessToken, actorURN, commentURN string) error {
	u := fmt.Sprintf(
		"%s/comments/%s?actor=%s",
		RestBaseURL, url.PathEscape(commentURN), url.QueryEscape(actorURN),
	)
	req, err := http.NewRequest(http.MethodDelete, u, nil)
	if err != nil {
		return fmt.Errorf("linkedin: build delete-comment request: %w", err)
	}
	restHeaders(req, accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("linkedin: delete-comment request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("linkedin: delete-comment returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
