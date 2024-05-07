package openai

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-resty/resty/v2"
)

type ISubmitRequest struct {
	ThreadId          string            `json:"threadId"`
	RunId             string            `json:"runId"`
	FunctionResponses _FunctionResponse `json:"functionResponses"`
}

type _FunctionResponse struct {
	ToolOutputs []ToolOutput `json:"tool_outputs"`
}

type ToolOutput struct {
	ToolCallId string `json:"tool_call_id"`
	Output     string `json:"output"`
}

func SubmitToolOutput(threadID string, runId string, toolOutputs []ToolOutput) (*IRunObject, error) {
	// Set up the REST client
	client := resty.New()

	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/threads/%s/runs/%s/submit_tool_outputs", baseURL, threadID, runId)

	requestBody := ISubmitRequest{
		ThreadId: threadID,
		RunId:    runId,
		FunctionResponses: _FunctionResponse{
			ToolOutputs: toolOutputs,
		},
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
