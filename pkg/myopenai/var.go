package myopenai

import (
	"log"
	"os"

	oai "github.com/openai/openai-go/v3" // imported as openai
	"github.com/openai/openai-go/v3/option"
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
	ArjunAssistant_v2 AssistantID = "asst_mIiUVRzX8IcBovzLfGhNQoh3"
	ArjunAssistant    AssistantID = "asst_JDXM0Tqx60PAdqrANtNvd5PB"
)

var Client oai.Client

func init() {
	envValue := os.Getenv("OPENAI_API_KEY")

	// Check if the environment variable is set
	if envValue == "" {
		log.Println("Environment variable is not set")
	} else {
		log.Println("Environment variable value:", envValue)
		apiKey = envValue
	}

	Client = oai.NewClient(
		option.WithAPIKey(envValue),
	)
}
