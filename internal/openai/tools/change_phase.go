package openaitools

import (
	"encoding/json"
	"log"

	openaifc "github.com/TrendsHub/th-backend/internal/openai/fc"
	"github.com/TrendsHub/th-backend/pkg/openai"
)

type ChangePhaseOuput struct {
	MissedInformation []string `json:"missed_information"`
	MissedPhases      []int    `json:"missed_phases"`
}

func ChangePhaseFn(toolOption openai.ToolCall) (*openai.ToolOutput, error) {
	log.Println("Requires Action", toolOption.ID, "\n", toolOption.Function.Name, toolOption.Function.Arguments)
	cp := &openaifc.ChangePhase{}
	err := cp.ParseJson(toolOption.Function.Arguments)
	if err != nil {
		// log.Printf("Error %s", err.Error())
		return nil, err
	}
	eOutput, err := cp.FindEmptyFields()
	if err != nil {
		// log.Printf("Error %s", err.Error())
		return nil, err
	}

	oData := ChangePhaseOuput{
		MissedInformation: eOutput,
	}

	// Calculate Missed Phase Data here

	b, err := json.Marshal(oData)
	if err != nil {
		return nil, err
	}
	oStr := string(b)
	log.Println("Output to be send", oStr)
	return &openai.ToolOutput{
		ToolCallId: toolOption.ID,
		Output:     oStr,
	}, nil
}
