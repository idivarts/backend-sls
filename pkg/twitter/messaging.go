package twitter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// DMEvent represents a single direct-message event (MessageCreate only).
type DMEvent struct {
	ID               string    `json:"id"`
	Text             string    `json:"text"`
	SenderID         string    `json:"sender_id"`
	DMConversationID string    `json:"dm_conversation_id"`
	CreatedAt        time.Time `json:"created_at"`
}

// dmEventPayload is the raw API shape for a DM event.
type dmEventPayload struct {
	ID               string `json:"id"`
	Text             string `json:"text"`
	SenderID         string `json:"sender_id"`
	DMConversationID string `json:"dm_conversation_id"`
	CreatedAt        string `json:"created_at"`
	EventType        string `json:"event_type"`
}

type dmEventsResponse struct {
	Data []dmEventPayload `json:"data"`
}

// GetDMEvents returns recent direct-message events, filtered to MessageCreate.
func GetDMEvents(accessToken string, maxResults int) ([]DMEvent, error) {
	q := url.Values{}
	q.Set("dm_event.fields", "created_at,sender_id,dm_conversation_id,text,event_type")
	q.Set("max_results", strconv.Itoa(maxResults))
	requestURL := fmt.Sprintf("%s/dm_events?%s", APIURL, q.Encode())

	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("twitter: failed to build dm_events request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitter: dm_events request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitter: dm_events returned %d: %s", resp.StatusCode, string(body))
	}

	var r dmEventsResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("twitter: failed to parse dm_events response: %w", err)
	}

	events := make([]DMEvent, 0, len(r.Data))
	for _, p := range r.Data {
		if p.EventType != "MessageCreate" {
			continue
		}
		e := DMEvent{
			ID:               p.ID,
			Text:             p.Text,
			SenderID:         p.SenderID,
			DMConversationID: p.DMConversationID,
		}
		if p.CreatedAt != "" {
			if parsed, err := time.Parse(time.RFC3339, p.CreatedAt); err == nil {
				e.CreatedAt = parsed
			}
		}
		events = append(events, e)
	}
	return events, nil
}

// GetUserByID returns a user's profile by id.
func GetUserByID(accessToken, userID string) (*User, error) {
	requestURL := fmt.Sprintf("%s/users/%s?user.fields=name,username,profile_image_url,public_metrics,description", APIURL, userID)

	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("twitter: failed to build get user request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitter: get user request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitter: get user returned %d: %s", resp.StatusCode, string(body))
	}

	var r struct {
		Data User `json:"data"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("twitter: failed to parse get user response: %w", err)
	}
	return &r.Data, nil
}

// SendDM sends a direct message to a single participant and returns the
// resulting dm_event_id.
func SendDM(accessToken, participantID, text string) (dmEventID string, err error) {
	payload, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return "", fmt.Errorf("twitter: failed to marshal dm body: %w", err)
	}

	requestURL := fmt.Sprintf("%s/dm_conversations/with/%s/messages", APIURL, participantID)
	req, err := http.NewRequest(http.MethodPost, requestURL, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("twitter: failed to build send dm request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("twitter: send dm request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("twitter: send dm returned %d: %s", resp.StatusCode, string(body))
	}

	var r struct {
		Data struct {
			DMEventID string `json:"dm_event_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return "", fmt.Errorf("twitter: failed to parse send dm response: %w", err)
	}
	if r.Data.DMEventID == "" {
		return "", fmt.Errorf("twitter: send dm returned no dm_event_id: %s", string(body))
	}
	return r.Data.DMEventID, nil
}
