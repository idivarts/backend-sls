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

	// Presence only. This runs at package init in every lambda that imports
	// the OpenAI client, so printing the value put a live API key in plaintext
	// into CloudWatch on every cold start, on dev and on production alike.
	if envValue == "" {
		log.Println("myopenai: OPENAI_API_KEY is not set")
	} else {
		log.Printf("myopenai: OPENAI_API_KEY set (%d chars)", len(envValue))
		apiKey = envValue
	}

	Client = oai.NewClient(
		option.WithAPIKey(envValue),
	)
}
