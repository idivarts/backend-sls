package myopenai

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-resty/resty/v2"
)

type FunctionName string

const (
	CanConversationEndFn FunctionName = "can_conversation_end"
	ChangePhaseFn        FunctionName = "change_phase"
)

type ToolFunction struct {
	Name      FunctionName `json:"name"`      // "name": "can_conversation_end"
	Arguments string       `json:"arguments"` // "arguments": "{"enagement":"10k","views":"30K","video_category":"GRWM","brand_category":"Fashion,Beauty"}"
}

type ToolCall struct {
	ID       string       `json:"id"`       // "id": "call_uE7UdWM7GcGZp7JZcwa1CmZz"
	Type     string       `json:"type"`     // "type": "function"
	Function ToolFunction `json:"function"` // "function": { "name": "can_conversation_end", "arguments": "..." }
}

type RequiredAction struct {
	Type              string `json:"type"` // "type": "submit_tool_outputs"
	SubmitToolOutputs struct {
		ToolCalls []ToolCall `json:"tool_calls"` // "tool_calls": [ { ... } ]
	} `json:"submit_tool_outputs"`
}

type TruncationStrategy struct {
	Type         string      `json:"type"`          // "type": "auto"
	LastMessages interface{} `json:"last_messages"` // "last_messages": null
}
type IRunStatus string

const (
	REQUIRES_ACTION_STATUS IRunStatus = "requires_action"
	PENDING_STATUS         IRunStatus = "pending"
	COMPLETED_STATUS       IRunStatus = "completed"
	EXPIRED_STATUS         IRunStatus = "expired"
)

type IRunObject struct {
	ID                  string             `json:"id" validate:"required"` // "id": "run_Hn1azQLK1W6qyzBUrum26sMe"
	Object              string             `json:"object"`                 // "object": "thread.run"
	CreatedAt           int64              `json:"created_at"`             // "created_at": 1715098953
	AssistantID         string             `json:"assistant_id"`           // "assistant_id": "asst_3rJKwjfT1VeXRh6KHLg4hQoM"
	ThreadID            string             `json:"thread_id"`              // "thread_id": "thread_QTEJomy76q5ykzhfRSxB8pIu"
	Status              IRunStatus         `json:"status"`                 // "status": "requires_action | pending | completed | expired"
	StartedAt           int64              `json:"started_at"`             // "started_at": 1715098954
	ExpiresAt           int64              `json:"expires_at"`             // "expires_at": 1715099553
	CancelledAt         interface{}        `json:"cancelled_at"`           // "cancelled_at": null
	FailedAt            interface{}        `json:"failed_at"`              // "failed_at": null
	CompletedAt         interface{}        `json:"completed_at"`           // "completed_at": null
	RequiredAction      RequiredAction     `json:"required_action"`        // "required_action": { "type": "submit_tool_outputs", "submit_tool_outputs": { ... } }
	LastError           interface{}        `json:"last_error"`             // "last_error": null
	Model               string             `json:"model"`                  // "model": "gpt-3.5-turbo"
	Instructions        string             `json:"instructions"`           // "instructions": "The below instructions are written in markdown language format. Please process accordingly ..."
	Tools               []ToolCall         `json:"tools"`                  // "tools": [ { "type": "function", "function": { ... } }, { "type": "function", "function": { ... } } ]
	ToolResources       map[string]string  `json:"tool_resources"`         // "tool_resources": {}
	Metadata            map[string]string  `json:"metadata"`               // "metadata": {}
	Temperature         float64            `json:"temperature"`            // "temperature": 1.0
	TopP                float64            `json:"top_p"`                  // "top_p": 1.0
	MaxCompletionTokens int                `json:"max_completion_tokens"`  // "max_completion_tokens": null
	MaxPromptTokens     int                `json:"max_prompt_tokens"`      // "max_prompt_tokens": null
	TruncationStrategy  TruncationStrategy `json:"truncation_strategy"`    // "truncation_strategy": { "type": "auto", "last_messages": null }
	IncompleteDetails   interface{}        `json:"incomplete_details"`     // "incomplete_details": null
	Usage               interface{}        `json:"usage"`                  // "usage": null
	ResponseFormat      string             `json:"response_format"`        // "response_format": "auto"
	ToolChoice          interface{}        `json:"tool_choice"`            // "tool_choice": "auto"
}

func StartRun(threadID string, assistantID AssistantID, additionalInstructions string, requireFunction string) (*IRunObject, error) {
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

	// require a function execution?
	if requireFunction != "" {
		requestBody["tool_choice"] = map[string]interface{}{
			"type": "function",
			"function": map[string]string{
				"name": requireFunction,
			},
		}
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

func GetRunStatus(threadID string, runId string) (*IRunObject, error) {
	// Set up the REST client
	client := resty.New()

	// Set the API endpoint
	apiURL := fmt.Sprintf("%s/threads/%s/runs/%s", baseURL, threadID, runId)

	// Make the API request
	resp, err := client.R().
		SetHeader("Authorization", "Bearer "+apiKey).
		SetHeader("Content-Type", "application/json").
		SetHeader("OpenAI-Beta", "assistants=v2").
		Get(apiURL)
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
