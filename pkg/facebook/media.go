package facebook

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
)

type IFBPostsParams struct {
	Count int
	// WithEngagement additionally requests like/comment/share summaries per post.
	// Used by analytics to rank top-performing posts.
	WithEngagement bool
}

type fbResponse struct {
	Data   []IPostData `json:"data"`
	Paging Paging      `json:"paging"`
}

// fbEngagementSummary matches the `summary` shape returned for
// likes.summary(true) / comments.summary(true).
type fbEngagementSummary struct {
	Summary struct {
		TotalCount int `json:"total_count"`
	} `json:"summary"`
}

type fbShares struct {
	Count int `json:"count"`
}

type IPostData struct {
	Message      string     `json:"message"`
	FullPicture  string     `json:"full_picture"`
	ID           string     `json:"id"`
	PermalinkURL string     `json:"permalink_url"`
	CreatedTime  CustomTime `json:"created_time"`

	// Engagement — only populated when IFBPostsParams.WithEngagement is set.
	Likes    *fbEngagementSummary `json:"likes,omitempty"`
	Comments *fbEngagementSummary `json:"comments,omitempty"`
	Shares   *fbShares            `json:"shares,omitempty"`
}

// LikeCount returns the post's like total (0 if not requested/available).
func (p IPostData) LikeCount() int {
	if p.Likes == nil {
		return 0
	}
	return p.Likes.Summary.TotalCount
}

// CommentCount returns the post's comment total (0 if not requested/available).
func (p IPostData) CommentCount() int {
	if p.Comments == nil {
		return 0
	}
	return p.Comments.Summary.TotalCount
}

// ShareCount returns the post's share total (0 if not requested/available).
func (p IPostData) ShareCount() int {
	if p.Shares == nil {
		return 0
	}
	return p.Shares.Count
}

func GetPosts(pageID, accessToken string, params IFBPostsParams) ([]IPostData, error) {
	// Set up the HTTP client
	client := http.Client{}

	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/%s/%s/posts", BaseURL, ApiVersion, pageID)

	// Create query parameters
	fields := "message,full_picture,id,permalink_url,created_time"
	if params.WithEngagement {
		fields += ",likes.summary(true),comments.summary(true),shares"
	}
	iParam := url.Values{}
	iParam.Set("fields", fields)
	iParam.Set("access_token", accessToken)

	if params.Count == 0 {
		params.Count = 10
	}
	iParam.Set("limit", strconv.Itoa(params.Count))

	allParams := iParam.Encode()
	log.Println("All Params:", allParams)

	// Combine base URL and query parameters
	apiURL = fmt.Sprintf("%s?%s", apiURL, allParams)

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
	data := fbResponse{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, err
	}
	return data.Data, nil
}

// GetPostByID fetches a single Facebook Page post with its engagement summaries
// (likes/comments/shares). Used for per-post basic analytics.
func GetPostByID(postID, accessToken string) (*IPostData, error) {
	iParam := url.Values{}
	iParam.Set("fields", "message,full_picture,id,permalink_url,created_time,likes.summary(true),comments.summary(true),shares")
	iParam.Set("access_token", accessToken)
	apiURL := fmt.Sprintf("%s/%s/%s?%s", BaseURL, ApiVersion, postID, iParam.Encode())

	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("Error: Unexpected status code - " + resp.Status + "\n" + string(body))
	}

	post := IPostData{}
	if err := json.Unmarshal(body, &post); err != nil {
		return nil, err
	}
	return &post, nil
}
