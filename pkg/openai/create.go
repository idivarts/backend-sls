package openai

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-resty/resty/v2"
)

type Thread struct {
	ID            string            `json:"id"`
	Object        string            `json:"object"`
	CreatedAt     int64             `json:"created_at"`
	Metadata      map[string]string `json:"metadata"`
	ToolResources map[string]string `json:"tool_resources"`
}

func CreateThread() (*Thread, error) {
	// Set up the REST client
	client := resty.New()

	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/threads", baseURL)

	// Make the API request
	resp, err := client.R().
		SetHeader("Authorization", "Bearer "+apiKey).
		SetHeader("Content-Type", "application/json").
		SetHeader("OpenAI-Beta", "assistants=v2").
		Post(apiURL)
	if err != nil {
		return nil, err // Return the error if request fails
	}

	// Check for non-200 status code
	if resp.StatusCode() != http.StatusOK {
		return nil, errors.New("Error: Unexpected status code - " + resp.Status())
	}

	data := Thread{}
	// Unmarshal the response body
	if err := json.Unmarshal(resp.Body(), &data); err != nil {
		return nil, err // Return any JSON unmarshal errors
	}

	return &data, nil
}
