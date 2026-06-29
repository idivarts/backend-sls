package reddit

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// Submission is a Reddit post (a t3 "link" thing).
type Submission struct {
	Fullname    string  `json:"fullname"` // t3_… fullname (from "name")
	ID          string  `json:"id"`       // base-36 id (without prefix)
	Title       string  `json:"title"`
	Permalink   string  `json:"permalink"` // absolute https://www.reddit.com/... URL
	URL         string  `json:"url"`       // link target (self-posts: the permalink)
	Subreddit   string  `json:"subreddit"`
	CreatedUTC  int64   `json:"created_utc"`
	Score       int64   `json:"score"`
	NumComments int64   `json:"num_comments"`
	UpvoteRatio float64 `json:"upvote_ratio"`
	Thumbnail   string  `json:"thumbnail"`
}

// Comment is a Reddit comment (a t1 thing).
type Comment struct {
	Fullname       string `json:"fullname"` // t1_… fullname (from "name")
	ID             string `json:"id"`
	Body           string `json:"body"`
	Author         string `json:"author"`
	AuthorFullname string `json:"author_fullname"` // t2_… account fullname
	CreatedUTC     int64  `json:"created_utc"`
	Score          int64  `json:"score"`
}

// listing is Reddit's standard listing envelope: { kind: "Listing", data: { children: [...] } }.
type listing struct {
	Kind string `json:"kind"`
	Data struct {
		Children []struct {
			Kind string          `json:"kind"`
			Data json.RawMessage `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

// GetUserSubmissions returns a user's recent posts. Requires the history (or
// read) scope. username is without the u/ prefix.
func GetUserSubmissions(accessToken, username string, limit int) ([]Submission, error) {
	if limit <= 0 {
		limit = 25
	}
	p := fmt.Sprintf("/user/%s/submitted?limit=%s&raw_json=1", url.PathEscape(username), itoa(limit))
	req, err := authedRequest(http.MethodGet, p, accessToken, nil)
	if err != nil {
		return nil, err
	}
	body, _, err := doAuthed(req)
	if err != nil {
		return nil, err
	}

	var l listing
	if err := json.Unmarshal(body, &l); err != nil {
		return nil, fmt.Errorf("reddit: parse submissions listing: %w", err)
	}

	out := make([]Submission, 0, len(l.Data.Children))
	for _, child := range l.Data.Children {
		if child.Kind != "t3" {
			continue
		}
		var raw struct {
			Name        string  `json:"name"`
			ID          string  `json:"id"`
			Title       string  `json:"title"`
			Permalink   string  `json:"permalink"`
			URL         string  `json:"url"`
			Subreddit   string  `json:"subreddit"`
			CreatedUTC  float64 `json:"created_utc"`
			Score       int64   `json:"score"`
			NumComments int64   `json:"num_comments"`
			UpvoteRatio float64 `json:"upvote_ratio"`
			Thumbnail   string  `json:"thumbnail"`
		}
		if err := json.Unmarshal(child.Data, &raw); err != nil {
			return nil, fmt.Errorf("reddit: parse submission: %w", err)
		}
		out = append(out, Submission{
			Fullname:    raw.Name,
			ID:          raw.ID,
			Title:       raw.Title,
			Permalink:   "https://www.reddit.com" + raw.Permalink,
			URL:         raw.URL,
			Subreddit:   raw.Subreddit,
			CreatedUTC:  int64(raw.CreatedUTC),
			Score:       raw.Score,
			NumComments: raw.NumComments,
			UpvoteRatio: raw.UpvoteRatio,
			Thumbnail:   raw.Thumbnail,
		})
	}
	return out, nil
}

// GetComments returns the top-level comments on a post. articleID is the t3 id
// WITHOUT the t3_ prefix. Requires the read scope. "more" placeholder children
// are skipped.
func GetComments(accessToken, articleID string) ([]Comment, error) {
	p := fmt.Sprintf("/comments/%s?raw_json=1&limit=200&depth=1", url.PathEscape(articleID))
	req, err := authedRequest(http.MethodGet, p, accessToken, nil)
	if err != nil {
		return nil, err
	}
	body, _, err := doAuthed(req)
	if err != nil {
		return nil, err
	}

	// The response is a 2-element array of Listings: [0] = the post, [1] = comments.
	var listings []listing
	if err := json.Unmarshal(body, &listings); err != nil {
		return nil, fmt.Errorf("reddit: parse comments response: %w", err)
	}
	if len(listings) < 2 {
		return []Comment{}, nil
	}

	commentListing := listings[1]
	out := make([]Comment, 0, len(commentListing.Data.Children))
	for _, child := range commentListing.Data.Children {
		if child.Kind != "t1" {
			// Skip "more" (kind "more") and any non-comment children.
			continue
		}
		var raw struct {
			Name           string  `json:"name"`
			ID             string  `json:"id"`
			Body           string  `json:"body"`
			Author         string  `json:"author"`
			AuthorFullname string  `json:"author_fullname"`
			CreatedUTC     float64 `json:"created_utc"`
			Score          int64   `json:"score"`
		}
		if err := json.Unmarshal(child.Data, &raw); err != nil {
			return nil, fmt.Errorf("reddit: parse comment: %w", err)
		}
		out = append(out, Comment{
			Fullname:       raw.Name,
			ID:             raw.ID,
			Body:           raw.Body,
			Author:         raw.Author,
			AuthorFullname: raw.AuthorFullname,
			CreatedUTC:     int64(raw.CreatedUTC),
			Score:          raw.Score,
		})
	}
	return out, nil
}

// ReplyToComment posts a reply to any thing (comment or post). parentFullname
// is a fullname (t1_… for a comment, t3_… for a post). Returns the new comment's
// fullname. Requires the submit scope.
func ReplyToComment(accessToken, parentFullname, text string) (commentFullname string, err error) {
	form := url.Values{}
	form.Set("thing_id", parentFullname)
	form.Set("text", text)
	body, err := formPost(accessToken, "/api/comment", form)
	if err != nil {
		return "", err
	}

	var env struct {
		JSON struct {
			Data struct {
				Things []struct {
					Data struct {
						Name string `json:"name"`
					} `json:"data"`
				} `json:"things"`
			} `json:"data"`
		} `json:"json"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return "", fmt.Errorf("reddit: parse comment response: %w", err)
	}
	if len(env.JSON.Data.Things) == 0 {
		return "", fmt.Errorf("reddit: comment response contained no things: %s", string(body))
	}
	return env.JSON.Data.Things[0].Data.Name, nil
}

// EditComment edits the body of an existing self-post or comment owned by the
// authenticated user. thingFullname is a t1_… or t3_… fullname. Requires the
// edit scope.
func EditComment(accessToken, thingFullname, text string) error {
	form := url.Values{}
	form.Set("thing_id", thingFullname)
	form.Set("text", text)
	_, err := formPost(accessToken, "/api/editusertext", form)
	return err
}

// DeleteThing deletes a post or comment owned by the authenticated user.
// fullname is a t1_…/t3_… fullname. Requires the edit scope. /api/del does not
// return a json envelope, so only the HTTP status is checked.
func DeleteThing(accessToken, fullname string) error {
	form := url.Values{}
	form.Set("id", fullname)
	_, err := formPost(accessToken, "/api/del", form)
	return err
}
