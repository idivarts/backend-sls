package openaitools

import (
	"log"

	sqsevents "github.com/idivarts/backend-sls/internal/message_sqs/events"
	"github.com/idivarts/backend-sls/internal/models"
	openaifc "github.com/idivarts/backend-sls/internal/openai/fc"
	"github.com/idivarts/backend-sls/pkg/myopenai"
)

type ChangePhaseOuput struct {
	MissedInformation []string `json:"missed_information"`
	MissedPhases      []int    `json:"missed_phases"`
}

func ChangePhaseFn(conv *sqsevents.ConversationEvent, toolOption myopenai.ToolCall, onlySimulate *models.Conversation) (*myopenai.ToolOutput, error) {
	log.Println("Requires Action:\n", toolOption.Function.Name, toolOption.Function.Arguments)
	cp := &openaifc.ChangePhase{}
	err := cp.ParseJson(toolOption.Function.Arguments)
	if err != nil {
		// log.Printf("Error %s", err.Error())
		return nil, err
	}

	cData := models.Conversation{}
	if onlySimulate != nil {
		cData = *onlySimulate
	} else {
		err = cData.GetByLead(conv.LeadID)
		if err != nil {
			return nil, err
		}
	}

	// TODO: Write code to calculate phase depending on collectibles
	// Copy the needed information to cData
	// if cp.Phase != 0 {
	// 	cData.Information.Phase = cp.Phase
	// }
	// if cp.InterestedInService != nil {
	// 	cData.Information.InterestedInService = cp.InterestedInService
	// }
	// if cp.Engagement != "" {
	// 	cData.Information.Engagement = cp.Engagement
	// }
	// if cp.EngagementUnit != "" {
	// 	cData.Information.EngagementUnit = cp.EngagementUnit
	// }
	// if cp.Views != "" {
	// 	cData.Information.Views = cp.Views
	// }
	// if cp.ViewsUnit != "" {
	// 	cData.Information.ViewsUnit = cp.ViewsUnit
	// }
	// if cp.BrandCategory != "" {
	// 	cData.Information.BrandCategory = cp.BrandCategory
	// }
	// if cp.VideoCategory != "" {
	// 	cData.Information.VideoCategory = cp.VideoCategory
	// }
	// if cp.InterestedInApp != nil {
	// 	cData.Information.InterestedInApp = cp.InterestedInApp
	// }
	// if cp.CollaborationBrand != "" {
	// 	cData.Information.CollaborationBrand = cp.CollaborationBrand
	// }
	// if cp.CollaborationProduct != "" {
	// 	cData.Information.CollaborationProduct = cp.CollaborationProduct
	// }

	// eOutput, err := cData.Information.FindEmptyFields()
	// if err != nil {
	// 	// log.Printf("Error %s", err.Error())
	// 	return nil, err
	// }

	// oData := ChangePhaseOuput{
	// 	MissedInformation: eOutput,
	// }

	// // Calculate Missed Phase Data here
	// if cp.Phase == 6 {
	// 	oData.MissedPhases = []int{}
	// } else {
	// 	if cData.Phases == nil {
	// 		cData.Phases = []int{1}
	// 	}
	// 	// Fetch data from dynamodb and calculate missed phase
	// 	if len(cData.Phases) > 0 && cData.Phases[len(cData.Phases)-1] != cp.Phase {
	// 		cData.Phases = append(cData.Phases, cp.Phase)
	// 	}
	// 	cData.CurrentPhase = cp.Phase

	// 	if cData.Information.InterestedInService == nil {
	// 		oData.MissedPhases = append(oData.MissedPhases, 1)
	// 	}
	// 	if cData.Information.Engagement == "" || cData.Information.Views == "" || cData.Information.BrandCategory == "" || cData.Information.VideoCategory == "" {
	// 		oData.MissedPhases = append(oData.MissedPhases, 2)
	// 	}
	// 	if cData.Information.InterestedInApp == nil {
	// 		oData.MissedPhases = append(oData.MissedPhases, 3)
	// 	}
	// 	if cData.Information.CollaborationProduct == "" || cData.Information.CollaborationBrand == "" {
	// 		oData.MissedPhases = append(oData.MissedPhases, 4)
	// 	}

	// }

	// if onlySimulate == nil {
	// 	_, err = cData.Insert()
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// }

	// b, err := json.Marshal(oData)
	// if err != nil {
	// 	return nil, err
	// }
	// oStr := string(b)
	// log.Println("Output to be send", oStr)
	return &myopenai.ToolOutput{
		ToolCallId: toolOption.ID,
		Output:     "", //oStr,
	}, nil
}
