package openai

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-resty/resty/v2"
)

type IRunObject struct {
	ID                  string                 `json:"id" validate:"required"`
	Object              string                 `json:"object"`
	CreatedAt           int64                  `json:"created_at"`
	AssistantID         string                 `json:"assistant_id"`
	ThreadID            string                 `json:"thread_id"`
	Status              string                 `json:"status"`
	StartedAt           interface{}            `json:"started_at"`
	ExpiresAt           int64                  `json:"expires_at"`
	CancelledAt         interface{}            `json:"cancelled_at"`
	FailedAt            interface{}            `json:"failed_at"`
	CompletedAt         interface{}            `json:"completed_at"`
	RequiredAction      interface{}            `json:"required_action"`
	LastError           interface{}            `json:"last_error"`
	Model               string                 `json:"model"`
	Instructions        string                 `json:"instructions"`
	Tools               []interface{}          `json:"tools"`
	ToolResources       map[string]interface{} `json:"tool_resources"`
	Metadata            map[string]interface{} `json:"metadata"`
	Temperature         float64                `json:"temperature"`
	TopP                float64                `json:"top_p"`
	MaxCompletionTokens interface{}            `json:"max_completion_tokens"`
	MaxPromptTokens     interface{}            `json:"max_prompt_tokens"`
	TruncationStrategy  struct {
		Type         string      `json:"type"`
		LastMessages interface{} `json:"last_messages"`
	} `json:"truncation_strategy"`
	IncompleteDetails interface{} `json:"incomplete_details"`
	Usage             interface{} `json:"usage"`
	ResponseFormat    string      `json:"response_format"`
	ToolChoice        string      `json:"tool_choice"`
}

func StartRun(threadID string, assistantID AssistantID, additionalInstructions string) (*IRunObject, error) {
	// Set up the REST client
	client := resty.New()

	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/threads/%s/runs", baseURL, threadID)

	// Create the request body
	requestBody := map[string]interface{}{
		"assistant_id": assistantID,
	}

	// additional_instructions
	if additionalInstructions != "" {
		requestBody["additional_instructions"] = additionalInstructions
	}

	// Make the API request
	resp, err := client.R().
		SetHeader("Authorization", "Bearer "+apiKey).
		SetHeader("Content-Type", "application/json").
		SetHeader("OpenAI-Beta", "assistants=v2").
		SetBody(requestBody).
		Post(apiURL)
	if err != nil {
		return nil, err // Return the error if request fails
	}

	// Check for non-200 status code
	if resp.StatusCode() != http.StatusOK {
		return nil, errors.New("Error: Unexpected status code - " + resp.Status())
	}

	data := &IRunObject{}
	// Unmarshal the response body
	if err := json.Unmarshal(resp.Body(), data); err != nil {
		return nil, err // Return any JSON unmarshal errors
	}

	return data, nil
}
