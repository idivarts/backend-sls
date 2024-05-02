package openai

import (
	"fmt"

	"github.com/go-resty/resty/v2"
)

func StartRun(threadID string, assistantID AssistantID) error {
	// Set up the REST client
	client := resty.New()

	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/threads/%s/runs", baseURL, threadID)

	// Create the request body
	requestBody := map[string]interface{}{
		"assistant_id": assistantID,
	}

	// Make the API request
	_, err := client.R().
		SetHeader("Authorization", "Bearer "+apiKey).
		SetHeader("Content-Type", "application/json").
		SetHeader("OpenAI-Beta", "assistants=v2").
		SetBody(requestBody).
		Post(apiURL)

	return err
}