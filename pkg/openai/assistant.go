package openai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// Define a struct for the request body
type CreateAssistantRequest struct {
	Name         string      `json:"name"`
	Model        string      `json:"model"`
	Instructions string      `json:"instructions"`
	Tools        []ToolEntry `json:"tools"`
}

type ToolType string

const (
	TT_FUNCTION ToolType = "function"
)

// Define a struct for ToolEntry
type ToolEntry struct {
	Type     ToolType `json:"type"`
	Function Function `json:"function"`
}

// Define a struct for Function
type Function struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Parameters  Parameters `json:"parameters"`
}

type ParameterType string

const (
	PT_OBJECT ParameterType = "object"
)

// Define struct for Parameters
type Parameters struct {
	Type       ParameterType               `json:"type"`
	Properties map[string]VariableProperty `json:"properties"`
	Required   []string                    `json:"required,omitempty"`
}

type VariableType string

const (
	VT_STRING  VariableType = "string"
	VT_NUMBER  VariableType = "number"
	VT_BOOLEAN VariableType = "boolean"
)

// Define struct for Unit Property
type VariableProperty struct {
	Type        VariableType `json:"type"`
	Enum        []string     `json:"enum,omitempty"`
	Description string       `json:"description"`
}

type AssistantReponse struct {
	ID string `json:"id"`
}

// Function to make the API call
func CreateAssistant(assistant CreateAssistantRequest) (*AssistantReponse, error) {
	// Marshal the body into JSON
	jsonBody, err := json.Marshal(assistant)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal body: %v", err)
	}

	url := fmt.Sprintf("%s/assistants", baseURL)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// Set appropriate headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("OpenAI-Beta", "assistants=v2")

	// Make the HTTP request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Read the response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("Error: Unexpected status code - " + resp.Status + "\n" + string(body))
	}

	fmt.Println("Response:", string(body))
	data := AssistantReponse{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, err
	}

	return &data, nil
}

// Function to make the API call
func UpdateAssistant(assistantID string, assistant CreateAssistantRequest) (*AssistantReponse, error) {
	// Marshal the body into JSON
	jsonBody, err := json.Marshal(assistant)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal body: %v", err)
	}

	url := fmt.Sprintf("%s/assistants/%s", baseURL, assistantID)

	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// Set appropriate headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("OpenAI-Beta", "assistants=v2")

	// Make the HTTP request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Read the response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("Error: Unexpected status code - " + resp.Status + "\n" + string(body))
	}

	fmt.Println("Response:", string(body))
	data := AssistantReponse{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, err
	}

	return &data, nil
}

// Function to make the API call
func DeleteAssistant(assistantID string) (*AssistantReponse, error) {

	url := fmt.Sprintf("%s/assistants/%s", baseURL, assistantID)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("OpenAI-Beta", "assistants=v2")

	// Make the HTTP request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Read the response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("Error: Unexpected status code - " + resp.Status + "\n" + string(body))
	}

	fmt.Println("Response:", string(body))
	data := AssistantReponse{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, err
	}

	return &data, nil
}
