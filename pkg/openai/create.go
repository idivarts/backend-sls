package openai

import (
	"fmt"

	"github.com/go-resty/resty/v2"
)

func CreateThread() error {
	// Set up the REST client
	client := resty.New()

	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/threads", baseURL)

	// Make the API request
	_, err := client.R().
		SetHeader("Authorization", "Bearer "+apiKey).
		SetHeader("Content-Type", "application/json").
		SetHeader("OpenAI-Beta", "assistants=v2").
		Post(apiURL)

	return err
}
