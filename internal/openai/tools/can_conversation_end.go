package openaitools

import (
	"log"

	openaifc "github.com/idivarts/backend-sls/internal/openai/fc"
	"github.com/idivarts/backend-sls/pkg/myopenai"
)

func CanConversationEnd(toolOption myopenai.ToolCall) (*myopenai.ToolOutput, error) {
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
	return &myopenai.ToolOutput{
		ToolCallId: toolOption.ID,
		Output:     *eOutput,
	}, nil
}
