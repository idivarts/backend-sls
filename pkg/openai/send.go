package openai

import (
	"encoding/json"
	"fmt"

	"github.com/go-resty/resty/v2"
)

type ISendMessage struct {
	ID          string      `json:"id" validate:"required"`
	Object      string      `json:"object"`
	CreatedAt   int64       `json:"created_at"`
	AssistantID interface{} `json:"assistant_id"` // Assuming AssistantID can be null or string
	ThreadID    string      `json:"thread_id"`
	RunID       interface{} `json:"run_id"` // Assuming RunID can be null or string
	Role        string      `json:"role"`
	Content     []struct {
		Type string `json:"type"`
		Text struct {
			Value       string        `json:"value"`
			Annotations []interface{} `json:"annotations"`
		} `json:"text"`
	} `json:"content"`
	Attachments []interface{}          `json:"attachments"`
	Metadata    map[string]interface{} `json:"metadata"`
}

func SendMessage(threadID string, message string, isAssistant bool) (*ISendMessage, error) {
	// Set up the REST client
	client := resty.New()

	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/threads/%s/messages", baseURL, threadID)

	role := "user"
	if isAssistant {
		role = "assistant"
	}
	// Create the request body
	requestBody := map[string]interface{}{
		"role":    role,
		"content": message,
	}

	// Make the API request
	resp, err := client.R().
		SetHeader("Authorization", "Bearer "+apiKey).
		SetHeader("Content-Type", "application/json").
		SetHeader("OpenAI-Beta", "assistants=v2").
		SetBody(requestBody).
		Post(apiURL)
	if err != nil {
		return nil, err
	}
	// data, err := io.ReadAll(resp.RawBody())
	// if err != nil {
	// 	return err
	// }

	// fmt.Println(string(resp.Body()))
	data := &ISendMessage{}
	err = json.Unmarshal(resp.Body(), data)
	if err != nil {
		return nil, err
	}

	return data, nil
}
