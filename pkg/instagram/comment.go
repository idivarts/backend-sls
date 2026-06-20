package instagram

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/idivarts/backend-sls/pkg/messenger"
)

type InstagramCommentFrom struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type InstagramComment struct {
	ID        string               `json:"id"`
	Text      string               `json:"text"`
	From      InstagramCommentFrom `json:"from,omitempty"`
	Timestamp messenger.CustomTime `json:"timestamp"`
	LikeCount int                  `json:"like_count"`
}

type commentResponse struct {
	Data []InstagramComment `json:"data"`
}

type IGetCommentsParams struct {
	GraphType int
	Count     int
}

func GetComments(mediaID, accessToken string, params IGetCommentsParams) ([]InstagramComment, error) {
	client := http.Client{}

	apiURL := fmt.Sprintf("%s/%s/%s/comments", BaseURL, ApiVersion, mediaID)
	if params.GraphType == 0 {
		apiURL = fmt.Sprintf("%s/%s/%s/comments", messenger.BaseURL, messenger.ApiVersion, mediaID)
	}

	iParam := url.Values{}
	iParam.Set("fields", "id,text,from{id,username},timestamp,like_count")
	iParam.Set("access_token", accessToken)

	if params.Count == 0 {
		params.Count = 2
	}
	iParam.Set("limit", strconv.Itoa(params.Count))

	apiURL = fmt.Sprintf("%s?%s", apiURL, iParam.Encode())

	resp, err := client.Get(apiURL)
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

	data := commentResponse{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	return data.Data, nil
}

type CommentReplyResponse struct {
	ID string `json:"id"`
}

// ReplyToComment posts a reply to an existing top-level comment.
// Docs: https://developers.facebook.com/docs/instagram-api/reference/ig-comment/replies
// graphType: 0 = Facebook Graph (page-scoped access token), non-zero = Instagram Graph.
func ReplyToComment(commentID, message, accessToken string, graphType int) (*CommentReplyResponse, error) {
	client := http.Client{}

	apiURL := fmt.Sprintf("%s/%s/%s/replies", BaseURL, ApiVersion, commentID)
	if graphType == 0 {
		apiURL = fmt.Sprintf("%s/%s/%s/replies", messenger.BaseURL, messenger.ApiVersion, commentID)
	}

	iParam := url.Values{}
	iParam.Set("message", message)
	iParam.Set("access_token", accessToken)

	apiURL = fmt.Sprintf("%s?%s", apiURL, iParam.Encode())

	resp, err := client.Post(apiURL, "application/x-www-form-urlencoded", nil)
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

	data := CommentReplyResponse{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	return &data, nil
}
