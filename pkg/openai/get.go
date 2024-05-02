package openai

import (
	"fmt"

	"github.com/go-resty/resty/v2"
)

func GetMessages(threadID string) error {
	// Set up the REST client
	client := resty.New()

	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/threads/%s/messages", baseURL, threadID)

	// Make the API request
	_, err := client.R().
		SetHeader("Authorization", "Bearer "+apiKey).
		SetHeader("Content-Type", "application/json").
		SetHeader("OpenAI-Beta", "assistants=v2").
		Get(apiURL)

	return err
}
