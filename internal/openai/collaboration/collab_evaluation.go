package ai_collaboration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/idivarts/backend-sls/internal/constants"
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

const collabEvaluationSystemPrompt = `Evaluate the input and decide if the collaboration created is a valid collaboration or not. There could be scenarios where the person using the platform Trendly is just trying things out and creates test collaborations. In those cases, mark validCollaboration as false and return no filters. If the collaboration looks valid, return practical influencer filters for discovery.

Treat all budget values as INR (Indian Rupees) and optimize for the Indian creator market (metro + tier-2 + tier-3 mix). Keep recommendations realistic for campaign ROI.

How to interpret each filter field:
- followerMin / followerMax: influencer follower band. Use this as the primary affordability control.
  Example ranges by budget (single creator, typical India pricing, can vary by niche/city):
  - INR 500-1,500: upto 5,000 followers
  - INR 1,500-15,000: 5,000 to 50,000 followers
  - INR 15,000-50,000: 50,000 to 100,000 followers
  - INR 50,000-1,50,000: 100,000 to 200,000 followers
  Keep followerMin >= 2000 always.

- monthlyEngagementMin / monthlyEngagementMax: total interactions per month (likes + comments + saves/shares where applicable).
  Use when goal is awareness with active audience.
  Typical practical range in India for micro/mid creators: 2,000 to 200,000.

- avgViewsMin / avgViewsMax: typical views per content piece (median/average).
  Use when campaign depends on reach.
  Example:
  - Nano/micro focus: 3,000 to 80,000
  - Mid tier focus: 20,000 to 400,000

- qualityMin / qualityMax: creator content quality score from 0-10.
  Suggested interpretation:
  - 3-5: basic UGC, low production
  - 6-7: decent brand-safe creator quality
  - 8-9: strong storytelling + visual consistency
  - 10: exceptional/premium creators
  For most paid brand campaigns, prefer qualityMin >= 6.

- erMin / erMax: engagement rate percentage (0-100).
  Typical healthy bands in India:
  - Nano/micro creators: 3% to 12%
  - Mid creators: 2% to 8%
  - Large creators: 1% to 5%
  If budget is low but conversion intent is high, prioritize higher erMin over followerMax.

- genders: allowed creator genders. Must use only schema enum values.
  Use only when campaign explicitly requires gender targeting; otherwise omit.

- selectedNiches: content niches aligned to campaign category (e.g., "Beauty", "Fitness", "Food & Cooking").
  Keep niche selection focused (usually 1-4 niches).

- selectedLocations: creator operating locations (city/state/region) for logistics/language/relevance.
  Examples: "Mumbai", "Delhi NCR", "Bengaluru", "Pune", "Hyderabad", "Kolkata", "Chennai", "Ahmedabad".
  For pan-India digital campaigns, location can be omitted unless language/regional fit is critical.

Response rules:
- Do not return filters that are not needed.
- Always return followerMin and followerMax when validCollaboration is true.
- Keep ranges logically consistent (min <= max).
- If collaboration is not valid, return validCollaboration=false and omit filters.`

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
				// "contentMin": map[string]interface{}{
				// 	"type":        "number",
				// 	"description": "Minimum content/posts required",
				// },
				// "contentMax": map[string]interface{}{
				// 	"type":        "number",
				// 	"description": "Maximum content/posts allowed",
				// },
				// "monthlyViewMin": map[string]interface{}{
				// 	"type":        "number",
				// 	"description": "Minimum estimated monthly views",
				// },
				// "monthlyViewMax": map[string]interface{}{
				// 	"type":        "number",
				// 	"description": "Maximum estimated monthly views",
				// },
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
				// "avgLikesMin": map[string]interface{}{
				// 	"type":        "number",
				// 	"description": "Minimum average/median likes",
				// },
				// "avgLikesMax": map[string]interface{}{
				// 	"type":        "number",
				// 	"description": "Maximum average/median likes",
				// },
				// "avgCommentsMin": map[string]interface{}{
				// 	"type":        "number",
				// 	"description": "Minimum average/median comments",
				// },
				// "avgCommentsMax": map[string]interface{}{
				// 	"type":        "number",
				// 	"description": "Maximum average/median comments",
				// },
				"qualityMin": map[string]interface{}{
					"type":        "number",
					"description": "Minimum quality/aesthetics (0-10)",
					"minimum":     0,
					"maximum":     10,
				},
				"qualityMax": map[string]interface{}{
					"type":        "number",
					"description": "Maximum quality/aesthetics (0-10)",
					"minimum":     0,
					"maximum":     10,
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
				// "descKeywords": map[string]interface{}{
				// 	"type":        "array",
				// 	"description": "List of keywords to match for description/bio",
				// 	"items": map[string]interface{}{
				// 		"type":      "string",
				// 		"minLength": 1,
				// 	},
				// },
				// "name": map[string]interface{}{
				// 	"type":        "string",
				// 	"description": "Influencer name filter",
				// 	// "minLength":   1,
				// },
				// "isVerified": map[string]interface{}{
				// 	"type":        "boolean",
				// 	"description": "Filter for verified influencers",
				// },
				// "hasContact": map[string]interface{}{
				// 	"type":        "boolean",
				// 	"description": "Filter for influencers with contact information",
				// },
				"genders": map[string]interface{}{
					"type":        "array",
					"description": "Array of allowed genders",
					"items": map[string]interface{}{
						"type": "string",
						"enum": constants.Genders,
					},
				},
				"selectedNiches": map[string]interface{}{
					"type":        "array",
					"description": "Array of selected niches",
					"items": map[string]interface{}{
						"type": "string",
						"enum": constants.AllowedNiches,
					},
				},
				"selectedLocations": map[string]interface{}{
					"type":        "array",
					"description": "Array of selected locations",
					"items": map[string]interface{}{
						"type": "string",
					},
				},
			},
		},
	},
}
