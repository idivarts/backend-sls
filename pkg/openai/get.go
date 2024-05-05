package openai

import (
	"encoding/json"
	"fmt"

	"github.com/go-resty/resty/v2"
)

type Message struct {
	ID          string `json:"id"`
	Object      string `json:"object"`
	CreatedAt   int64  `json:"created_at"`
	AssistantID string `json:"assistant_id,omitempty"`
	ThreadID    string `json:"thread_id"`
	RunID       string `json:"run_id,omitempty"`
	Role        string `json:"role"`
	Content     []struct {
		Type string `json:"type"`
		Text struct {
			Value       string   `json:"value"`
			Annotations []string `json:"annotations"`
		} `json:"text"`
	} `json:"content"`
	Attachments []interface{} `json:"attachments"`
	Metadata    struct{}      `json:"metadata"`
}

type ListData struct {
	Object  string    `json:"object"`
	Data    []Message `json:"data"`
	FirstID string    `json:"first_id"`
	LastID  string    `json:"last_id"`
	HasMore bool      `json:"has_more"`
}

func GetMessages(threadID string, limit int, runId string) (*ListData, error) {
	// Set up the REST client
	client := resty.New()

	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/threads/%s/messages?limit=%d", baseURL, threadID, limit)

	if runId != "" {
		apiURL = fmt.Sprintf("%s/threads/%s/messages?run_id=%s", baseURL, threadID, runId)
	}
	// Make the API request
	resp, err := client.R().
		SetHeader("Authorization", "Bearer "+apiKey).
		SetHeader("Content-Type", "application/json").
		SetHeader("OpenAI-Beta", "assistants=v2").
		Get(apiURL)
	if err != nil {
		return nil, err
	}

	// d, err := io.ReadAll(resp.RawBody())
	// if err != nil {
	// 	return nil, err
	// }

	data := ListData{}
	err = json.Unmarshal(resp.Body(), &data)
	if err != nil {
		return nil, err
	}
	return &data, nil
}
