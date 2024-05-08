package openai

import (
	"log"
	"os"
)

const (
	baseURL = "https://api.openai.com/v1"
)

var (
	apiKey = "sk-proj-jx7xhhAMe27SKaDGMKr8T3BlbkFJazp4XlPOqap2HHSU3ttH"
)

type AssistantID string

const (
	ArjunAssistant_v1 AssistantID = "asst_3rJKwjfT1VeXRh6KHLg4hQoM"
	ArjunAssistant    AssistantID = "asst_mIiUVRzX8IcBovzLfGhNQoh3"
)

func init() {
	envValue := os.Getenv("OPENAI_API_KEY")

	// Check if the environment variable is set
	if envValue == "" {
		log.Println("Environment variable is not set")
	} else {
		log.Println("Environment variable value:", envValue)
		apiKey = envValue
	}

}
