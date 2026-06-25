package instagram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/idivarts/backend-sls/pkg/facebook"
)

// Messaging + comment operations for Instagram accounts connected via Instagram
// Login (graph.instagram.com) using an IG user access token.
//
// For IG Business Accounts linked to a Facebook Page, use the page-token helpers
// in pkg/facebook instead (those go through graph.facebook.com).

// ── Direct messages ───────────────────────────────────────────────────────────

type igSendMessage struct {
	Recipient struct {
		ID string `json:"id"`
	} `json:"recipient"`
	Message struct {
		Text string `json:"text"`
	} `json:"message"`
}

// SendIGMessage sends a DM reply from the connected IG account to a recipient.
// POST graph.instagram.com/{version}/me/messages
func SendIGMessage(recipientID, text, accessToken string) (*facebook.IMessageResponse, error) {
	payload := igSendMessage{}
	payload.Recipient.ID = recipientID
	payload.Message.Text = text

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf("%s/%s/me/messages?access_token=%s", BaseURL, ApiVersion, url.QueryEscape(accessToken))
	resp, err := http.Post(endpoint, "application/json", bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("SendIGMessage: status %d: %s", resp.StatusCode, string(body))
	}
	out := &facebook.IMessageResponse{}
	_ = json.Unmarshal(body, out)
	return out, nil
}

// GetIGConversations lists DM conversations for the connected IG account.
// GET graph.instagram.com/{version}/me/conversations?platform=instagram&fields=...
func GetIGConversations(accessToken string) (*facebook.ConversationData, error) {
	fields := "name,id,participants,messages{id,created_time,from,to,message,attachments{id,mime_type,name,file_url,image_data,video_data}}"
	endpoint := fmt.Sprintf("%s/%s/me/conversations?platform=instagram&fields=%s&access_token=%s",
		BaseURL, ApiVersion, url.QueryEscape(fields), url.QueryEscape(accessToken))
	resp, err := http.Get(endpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GetIGConversations: status %d: %s", resp.StatusCode, string(body))
	}
	data := &facebook.ConversationData{}
	if err := json.Unmarshal(body, data); err != nil {
		return nil, err
	}
	return data, nil
}

// GetUser fetches a DM contact's profile (name, username, profile picture) for an
// Instagram-Login account. The contact id is the Instagram-scoped id (IGSID) seen
// in conversation participants / webhook sender ids.
// GET graph.instagram.com/{version}/{igsid}?fields=name,username,profile_pic
func GetUser(igsid, accessToken string) (*facebook.UserProfile, error) {
	fields := "name,username,profile_pic"
	endpoint := fmt.Sprintf("%s/%s/%s?fields=%s&access_token=%s",
		BaseURL, ApiVersion, igsid, url.QueryEscape(fields), url.QueryEscape(accessToken))
	resp, err := http.Get(endpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GetUser: status %d: %s", resp.StatusCode, string(body))
	}
	out := &facebook.UserProfile{}
	if err := json.Unmarshal(body, out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetInstagramByUsername fetches a professional account's public profile via the
// Business Discovery API for an Instagram-Login connected account. selfIGID is the
// connected account's own IG id (the query node); accessToken is its IG user token.
// Used as a fallback because the messaging User Profile API withholds name/avatar
// for professional accounts. Returns data only for professional target accounts.
// GET graph.instagram.com/{version}/{selfIGID}?fields=business_discovery.username(<username>){...}
func GetInstagramByUsername(selfIGID, username, accessToken string) (*facebook.InstagramProfile, error) {
	fields := fmt.Sprintf("business_discovery.username(%s){id,name,username,profile_picture_url,followers_count}", username)
	endpoint := fmt.Sprintf("%s/%s/%s?fields=%s&access_token=%s",
		BaseURL, ApiVersion, selfIGID, url.QueryEscape(fields), url.QueryEscape(accessToken))

	resp, err := http.Get(endpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GetInstagramByUsername: status %d: %s", resp.StatusCode, string(body))
	}
	var out struct {
		BusinessDiscovery facebook.InstagramProfile `json:"business_discovery"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return &out.BusinessDiscovery, nil
}

// ── Comments ──────────────────────────────────────────────────────────────────

type createIDResponse struct {
	ID string `json:"id"`
}

// ReplyToIGComment posts a public reply under an IG comment.
// POST graph.instagram.com/{version}/{comment-id}/replies?message=...
func ReplyToIGComment(commentID, message, accessToken string) (string, error) {
	endpoint := fmt.Sprintf("%s/%s/%s/replies", BaseURL, ApiVersion, commentID)
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
		return "", fmt.Errorf("ReplyToIGComment: status %d: %s", resp.StatusCode, string(body))
	}
	var out createIDResponse
	_ = json.Unmarshal(body, &out)
	return out.ID, nil
}

// SetIGCommentHidden hides or unhides an IG comment.
// POST graph.instagram.com/{version}/{comment-id}?hide=true|false
func SetIGCommentHidden(commentID string, hidden bool, accessToken string) error {
	endpoint := fmt.Sprintf("%s/%s/%s", BaseURL, ApiVersion, commentID)
	form := url.Values{}
	form.Set("hide", fmt.Sprintf("%t", hidden))
	form.Set("access_token", accessToken)

	resp, err := http.PostForm(endpoint, form)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("SetIGCommentHidden: status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// DeleteIGObject deletes an IG object (comment) by id.
// DELETE graph.instagram.com/{version}/{object-id}
func DeleteIGObject(objectID, accessToken string) error {
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
		return fmt.Errorf("DeleteIGObject: status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
