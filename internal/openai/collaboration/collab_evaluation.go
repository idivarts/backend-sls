package ai_collaboration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/myopenai"
	"github.com/openai/openai-go/v3"
)

type CollabEvaluationInput struct {
	CollaborationName        string
	CollaborationDescription string
	Budget                   string
	Location                 string
	Questions                string
	Links                    string
	BrandDetails             string
}

type CollabEvaluationResponse struct {
	ValidCollaboration bool                               `json:"validCollaboration"`
	Filters            *trendlymodels.DiscoverPreferences `json:"filters,omitempty"`
}

func EvaluateCollaboration(input CollabEvaluationInput) (bool, *trendlymodels.DiscoverPreferences, error) {
	model := "gpt-4o-2024-08-06"
	ctx := context.Background()

	schemaParam := openai.ResponseFormatJSONSchemaJSONSchemaParam{
		Name:        "influencer_collaboration_evaluation",
		Description: openai.String("Evaluates collaboration validity and optional influencer discover filters"),
		Schema:      collabEvaluationJSONSchema,
		Strict:      openai.Bool(false),
	}

	userPrompt := fmt.Sprintf(`Evaluate this Collaboration -

Collaboration Name: %s
Collaboration Description: %s
Budget: %s
Location: %s

Questions to Infuencers: %s
External Links: %s

Other Brand Details - %s`,
		input.CollaborationName,
		input.CollaborationDescription,
		input.Budget,
		input.Location,
		input.Questions,
		input.Links,
		input.BrandDetails,
	)

	chat, err := myopenai.Client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(collabEvaluationSystemPrompt),
			openai.UserMessage(userPrompt),
		},
		Model: openai.ChatModel(model),
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
				JSONSchema: schemaParam,
			},
		},
	})
	if err != nil {
		log.Println("Error evaluating collaboration:", err.Error())
		return false, nil, err
	}

	if len(chat.Choices) == 0 {
		return false, nil, errors.New("no response from model")
	}

	rawJSON := chat.Choices[0].Message.Content
	var result CollabEvaluationResponse
	if err := json.Unmarshal([]byte(rawJSON), &result); err != nil {
		log.Println("Error parsing collaboration evaluation response:", err.Error())
		return false, nil, err
	}

	return result.ValidCollaboration, result.Filters, nil
}

const collabEvaluationSystemPrompt = `Evaluate the input and decide if the collaboration created is a valid collaboration or not. There could be scenarios where the person using the platform Trendly, they are just trying out and hence are creating just test collaborations. In that cases, mark validCollaboration as false and return no filters. However, if the collaboration looks like a valid one, suggest some filters that might be a good fit for searching the correct influencer for the collaboration the brand is looking for.

Note: while returning the filters, understand the budget and don't return filters like follower count, which is not feasible in the current budget.

Also, whatever filters you don't want to apply, simply don't return them in the response. The only exception is followers min and max. Ideally, min should never be below 2000`

var collabEvaluationJSONSchema = map[string]interface{}{
	"type":                 "object",
	"required":             []string{"validCollaboration"},
	"additionalProperties": false,
	"properties": map[string]interface{}{
		"validCollaboration": map[string]interface{}{
			"type":        "boolean",
			"description": "Indicates if the collaboration input is valid",
		},
		"filters": map[string]interface{}{
			"type":                 "object",
			"description":          "The influencer filters applied after evaluating the collaboration input",
			"additionalProperties": false,
			"required":             []string{"followerMin", "followerMax"},
			"properties": map[string]interface{}{
				"followerMin": map[string]interface{}{
					"type":        "number",
					"description": "Minimum followers required",
				},
				"followerMax": map[string]interface{}{
					"type":        "number",
					"description": "Maximum followers allowed",
				},
				"contentMin": map[string]interface{}{
					"type":        "number",
					"description": "Minimum content/posts required",
				},
				"contentMax": map[string]interface{}{
					"type":        "number",
					"description": "Maximum content/posts allowed",
				},
				"monthlyViewMin": map[string]interface{}{
					"type":        "number",
					"description": "Minimum estimated monthly views",
				},
				"monthlyViewMax": map[string]interface{}{
					"type":        "number",
					"description": "Maximum estimated monthly views",
				},
				"monthlyEngagementMin": map[string]interface{}{
					"type":        "number",
					"description": "Minimum estimated monthly engagements",
				},
				"monthlyEngagementMax": map[string]interface{}{
					"type":        "number",
					"description": "Maximum estimated monthly engagements",
				},
				"avgViewsMin": map[string]interface{}{
					"type":        "number",
					"description": "Minimum average/median views",
				},
				"avgViewsMax": map[string]interface{}{
					"type":        "number",
					"description": "Maximum average/median views",
				},
				"avgLikesMin": map[string]interface{}{
					"type":        "number",
					"description": "Minimum average/median likes",
				},
				"avgLikesMax": map[string]interface{}{
					"type":        "number",
					"description": "Maximum average/median likes",
				},
				"avgCommentsMin": map[string]interface{}{
					"type":        "number",
					"description": "Minimum average/median comments",
				},
				"avgCommentsMax": map[string]interface{}{
					"type":        "number",
					"description": "Maximum average/median comments",
				},
				"qualityMin": map[string]interface{}{
					"type":        "number",
					"description": "Minimum quality/aesthetics (0-100)",
					"minimum":     0,
					"maximum":     100,
				},
				"qualityMax": map[string]interface{}{
					"type":        "number",
					"description": "Maximum quality/aesthetics (0-100)",
					"minimum":     0,
					"maximum":     100,
				},
				"erMin": map[string]interface{}{
					"type":        "number",
					"description": "Minimum engagement rate percentage",
					"minimum":     0,
					"maximum":     100,
				},
				"erMax": map[string]interface{}{
					"type":        "number",
					"description": "Maximum engagement rate percentage",
					"minimum":     0,
					"maximum":     100,
				},
				"descKeywords": map[string]interface{}{
					"type":        "array",
					"description": "List of keywords to match for description",
					"items": map[string]interface{}{
						"type":      "string",
						"minLength": 1,
					},
				},
				"name": map[string]interface{}{
					"type":        "string",
					"description": "Influencer name filter",
					"minLength":   1,
				},
				"isVerified": map[string]interface{}{
					"type":        "boolean",
					"description": "Filter for verified influencers",
				},
				"hasContact": map[string]interface{}{
					"type":        "boolean",
					"description": "Filter for influencers with contact information",
				},
				"genders": map[string]interface{}{
					"type":        "array",
					"description": "Array of allowed genders",
					"items": map[string]interface{}{
						"type":      "string",
						"minLength": 1,
					},
				},
				"selectedNiches": map[string]interface{}{
					"type":        "array",
					"description": "Array of selected niches",
					"items": map[string]interface{}{
						"type":      "string",
						"minLength": 1,
					},
				},
				"selectedLocations": map[string]interface{}{
					"type":        "array",
					"description": "Array of selected locations",
					"items": map[string]interface{}{
						"type":      "string",
						"minLength": 1,
					},
				},
			},
		},
	},
}
