package openai

import (
	"fmt"

	"github.com/go-resty/resty/v2"
)

func SendMessage(threadID string, message string) error {
	// Set up the REST client
	client := resty.New()

	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/threads/%s/messages", baseURL, threadID)

	// Create the request body
	requestBody := map[string]interface{}{
		"role":    "user",
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
		return err
	}
	// data, err := io.ReadAll(resp.RawBody())
	// if err != nil {
	// 	return err
	// }
	fmt.Println(string(resp.Body()))

	return err
}
