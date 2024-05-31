package openaitools

import (
	"encoding/json"
	"log"

	sqsevents "github.com/TrendsHub/th-backend/internal/message_sqs/events"
	"github.com/TrendsHub/th-backend/internal/models"
	openaifc "github.com/TrendsHub/th-backend/internal/openai/fc"
	"github.com/TrendsHub/th-backend/pkg/openai"
)

type ChangePhaseOuput struct {
	MissedInformation []string `json:"missed_information"`
	MissedPhases      []int    `json:"missed_phases"`
}

func ChangePhaseFn(conv *sqsevents.ConversationEvent, toolOption openai.ToolCall) (*openai.ToolOutput, error) {
	log.Println("Requires Action:\n", toolOption.Function.Name, toolOption.Function.Arguments)
	cp := &openaifc.ChangePhase{}
	err := cp.ParseJson(toolOption.Function.Arguments)
	if err != nil {
		// log.Printf("Error %s", err.Error())
		return nil, err
	}

	cData := models.Conversation{}
	err = cData.Get(conv.IGSID)
	if err != nil {
		return nil, err
	}

	// Copy the needed information to cData
	if cp.Phase != 0 {
		cData.Information.Phase = cp.Phase
	}
	if cp.InterestedInService != nil {
		cData.Information.InterestedInService = cp.InterestedInService
	}
	if cp.Engagement != "" {
		cData.Information.Engagement = cp.Engagement
	}
	if cp.EngagementUnit != "" {
		cData.Information.EngagementUnit = cp.EngagementUnit
	}
	if cp.Views != "" {
		cData.Information.Views = cp.Views
	}
	if cp.ViewsUnit != "" {
		cData.Information.ViewsUnit = cp.ViewsUnit
	}
	if cp.BrandCategory != "" {
		cData.Information.BrandCategory = cp.BrandCategory
	}
	if cp.VideoCategory != "" {
		cData.Information.VideoCategory = cp.VideoCategory
	}
	if cp.InterestedInApp != nil {
		cData.Information.InterestedInApp = cp.InterestedInApp
	}
	if cp.CollaborationBrand != "" {
		cData.Information.CollaborationBrand = cp.CollaborationBrand
	}
	if cp.CollaborationProduct != "" {
		cData.Information.CollaborationProduct = cp.CollaborationProduct
	}

	eOutput, err := cData.Information.FindEmptyFields()
	if err != nil {
		// log.Printf("Error %s", err.Error())
		return nil, err
	}

	oData := ChangePhaseOuput{
		MissedInformation: eOutput,
	}

	// Calculate Missed Phase Data here
	if cp.Phase == 6 {
		oData.MissedPhases = []int{}
	} else {
		if cData.Phases == nil {
			cData.Phases = []int{1}
		}
		// Fetch data from dynamodb and calculate missed phase
		if len(cData.Phases) > 0 && cData.Phases[len(cData.Phases)-1] != cp.Phase {
			cData.Phases = append(cData.Phases, cp.Phase)
		}
		cData.CurrentPhase = cp.Phase

		if cData.Information.InterestedInService == nil {
			oData.MissedPhases = append(oData.MissedPhases, 1)
		}
		if cData.Information.Engagement == "" || cData.Information.Views == "" || cData.Information.BrandCategory == "" || cData.Information.VideoCategory == "" {
			oData.MissedPhases = append(oData.MissedPhases, 2)
		}
		if cData.Information.InterestedInApp == nil {
			oData.MissedPhases = append(oData.MissedPhases, 3)
		}
		if cData.Information.CollaborationProduct == "" || cData.Information.CollaborationBrand == "" {
			oData.MissedPhases = append(oData.MissedPhases, 4)
		}

	}
	_, err = cData.Insert()
	if err != nil {
		return nil, err
	}

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

func ChangePhaseSimulateFn(cData *models.Conversation, conv *sqsevents.ConversationEvent, toolOption openai.ToolCall) (*openai.ToolOutput, error) {
	log.Println("Requires Action:\n", toolOption.Function.Name, toolOption.Function.Arguments)
	cp := &openaifc.ChangePhase{}
	err := cp.ParseJson(toolOption.Function.Arguments)
	if err != nil {
		// log.Printf("Error %s", err.Error())
		return nil, err
	}

	// cData := models.Conversation{}
	// err = cData.Get(conv.IGSID)
	// if err != nil {
	// 	return nil, err
	// }

	// Copy the needed information to cData
	if cp.Phase != 0 {
		cData.Information.Phase = cp.Phase
	}
	if cp.InterestedInService != nil {
		cData.Information.InterestedInService = cp.InterestedInService
	}
	if cp.Engagement != "" {
		cData.Information.Engagement = cp.Engagement
	}
	if cp.EngagementUnit != "" {
		cData.Information.EngagementUnit = cp.EngagementUnit
	}
	if cp.Views != "" {
		cData.Information.Views = cp.Views
	}
	if cp.ViewsUnit != "" {
		cData.Information.ViewsUnit = cp.ViewsUnit
	}
	if cp.BrandCategory != "" {
		cData.Information.BrandCategory = cp.BrandCategory
	}
	if cp.VideoCategory != "" {
		cData.Information.VideoCategory = cp.VideoCategory
	}
	if cp.InterestedInApp != nil {
		cData.Information.InterestedInApp = cp.InterestedInApp
	}
	if cp.CollaborationBrand != "" {
		cData.Information.CollaborationBrand = cp.CollaborationBrand
	}
	if cp.CollaborationProduct != "" {
		cData.Information.CollaborationProduct = cp.CollaborationProduct
	}

	eOutput, err := cData.Information.FindEmptyFields()
	if err != nil {
		// log.Printf("Error %s", err.Error())
		return nil, err
	}

	oData := ChangePhaseOuput{
		MissedInformation: eOutput,
	}

	// Calculate Missed Phase Data here
	if cp.Phase == 6 {
		oData.MissedPhases = []int{}
	} else {
		if cData.Phases == nil {
			cData.Phases = []int{1}
		}
		// Fetch data from dynamodb and calculate missed phase
		if len(cData.Phases) > 0 && cData.Phases[len(cData.Phases)-1] != cp.Phase {
			cData.Phases = append(cData.Phases, cp.Phase)
		}
		cData.CurrentPhase = cp.Phase

		if cData.Information.InterestedInService == nil {
			oData.MissedPhases = append(oData.MissedPhases, 1)
		}
		if cData.Information.Engagement == "" || cData.Information.Views == "" || cData.Information.BrandCategory == "" || cData.Information.VideoCategory == "" {
			oData.MissedPhases = append(oData.MissedPhases, 2)
		}
		if cData.Information.InterestedInApp == nil {
			oData.MissedPhases = append(oData.MissedPhases, 3)
		}
		if cData.Information.CollaborationProduct == "" || cData.Information.CollaborationBrand == "" {
			oData.MissedPhases = append(oData.MissedPhases, 4)
		}

	}
	// _, err = cData.Insert()
	// if err != nil {
	// 	return nil, err
	// }

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
