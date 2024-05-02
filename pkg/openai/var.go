package openai

import (
	"log"
	"os"
)

const (
	baseURL = "https://api.openai.com/v1"
)

var (
	apiKey = "your_openai_api_key_here"
)

type AssistantID string

const (
	ArjunAssistant AssistantID = "asst_3rJKwjfT1VeXRh6KHLg4hQoM"
)

func init() {
	envValue := os.Getenv("OPENAI_API_KEY")

	// Check if the environment variable is set
	if envValue == "" {
		log.Println("Environment variable is not set")
	} else {
		log.Println("Environment variable value:", envValue)
	}

	apiKey = envValue
}
