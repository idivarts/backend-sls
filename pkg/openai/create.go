package openai

import (
	"encoding/json"
	"fmt"

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
		return nil, err
	}

	// d, err := io.ReadAll(resp.RawBody())
	// if err != nil {
	// 	return nil, err
	// }

	data := Thread{}
	err = json.Unmarshal(resp.Body(), &data)
	if err != nil {
		return nil, err
	}

	return &data, nil
}
