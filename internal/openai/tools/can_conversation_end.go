package openaitools

import (
	"log"

	openaifc "github.com/TrendsHub/th-backend/internal/openai/fc"
	"github.com/TrendsHub/th-backend/pkg/openai"
)

func CanConversationEnd(toolOption openai.ToolCall) (*openai.ToolOutput, error) {
	log.Println("Requires Action", toolOption.ID, "\n", toolOption.Function.Name, toolOption.Function.Arguments)
	cce := &openaifc.CanConversationEnd{}
	err := cce.ParseJson(toolOption.Function.Arguments)
	if err != nil {
		// log.Printf("Error %s", err.Error())
		return nil, err
	}
	eOutput, err := cce.FindEmptyFields()
	if err != nil {
		// log.Printf("Error %s", err.Error())
		return nil, err
	}
	log.Println("Output to be send", *eOutput)
	return &openai.ToolOutput{
		ToolCallId: toolOption.ID,
		Output:     *eOutput,
	}, nil
}
