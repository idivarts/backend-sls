package deduce

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/idivarts/backend-sls/internal/constants"
	"github.com/idivarts/backend-sls/pkg/myopenai"
	"github.com/openai/openai-go/v3"
)

// EnrichmentResult is the structured output returned by the LLM.
// It contains the deduced influencer attributes.
type EnrichmentResult struct {
	Gender    string   `json:"gender"`
	Location  string   `json:"location"`
	Niches    []string `json:"niches"`
	SubNiches []string `json:"subNiches"`
	Quality   int      `json:"quality"`
}

// EnrichInfluencer takes raw influencer information (bio, posts, etc.) as a
// string and returns a deduced EnrichmentResult using OpenAI structured outputs.
func EnrichInfluencer(influencerInfo string) (*EnrichmentResult, error) {
	model := "gpt-5-mini"

	ctx := context.Background()

	schemaParam := openai.ResponseFormatJSONSchemaJSONSchemaParam{
		Name:        "enrichment_result",
		Description: openai.String("Deduced influencer enrichment data including gender, location, niches, sub-niches, and quality rating"),
		Schema:      enrichmentJSONSchema,
		Strict:      openai.Bool(true),
	}

	chat, err := myopenai.Client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(enrichSystemPrompt),
			openai.UserMessage(influencerInfo),
		},
		Model: openai.ChatModel(model),
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
				JSONSchema: schemaParam,
			},
		},
	})

	if err != nil {
		log.Println("Error enriching influencer:", err.Error())
		return nil, fmt.Errorf("failed to generate content: %w", err)
	}

	if len(chat.Choices) == 0 {
		return nil, errors.New("no response from model")
	}

	rawJSON := chat.Choices[0].Message.Content

	var result EnrichmentResult
	if err := json.Unmarshal([]byte(rawJSON), &result); err != nil {
		log.Println("Error parsing structured response:", err.Error())
		return nil, fmt.Errorf("failed to unmarshal result: %w\nRaw response: %s", err, rawJSON)
	}

	return &result, nil
}

// enrichSystemPrompt is sent as the system message to guide the LLM.
const enrichSystemPrompt = `You are an expert Social Media Auditor. Your job is to analyze influencer data and return a JSON object.

Fields to deduce:
- gender: string (Deduce from Full Name, Username, and Bio/pronouns)
- location: string (Deduce from Bio and Posts' location/geo-tags)
- niches: string array (Deduce from Bio, Post Content, and Hashtags) — You MUST have at least one niche to pick values from the predefined niche list provided in the schema enum. Choose the closest matching niche(s). If nothing fits, put the niche as "Others".
- subNiches: string array (Optional, free-form sub-niches that provide finer detail beyond the broad niches above). Rules:
  * Sub-niches are NOT required. If the broad niches already capture the influencer well, return an empty array.
  * Be very selective — only include a sub-niche when it adds genuinely important specificity that the parent niche alone does not convey (e.g. "Korean Skincare" under Skincare, "Powerlifting" under Fitness, "Street Photography" under Photography).
  * Maximum 5 sub-niches, but typically 0, 1, or 2. Prefer fewer over more.
  * Each sub-niche should be a concise, descriptive label (2-4 words max).
  * Do NOT repeat or rephrase the parent niche — sub-niches must add new information.
- quality: integer (1-10, maps to a 5-star rating with half-star granularity)
  1: Poor (0.5 star)
  2: Below Average (1 star)
  3: Average (1.5 stars)
  4: Above Average (2 stars)
  5: Good (2.5 stars)
  6: Very Good (3 stars)
  7: Excellent (3.5 stars)
  8: Outstanding (4 stars)
  9: Exceptional (4.5 stars)
  10: Legendary (5 stars)
  (Determine based on Bio quality, Post caption depth, and Engagement consistency)

Return ONLY a JSON object matching the EnrichmentResult schema.`

// buildEnrichmentJSONSchema constructs the JSON schema dynamically so the
// niches enum always stays in sync with the AllowedNiches slice.
func buildEnrichmentJSONSchema() map[string]interface{} {
	nicheEnum := make([]interface{}, len(constants.AllowedNiches))
	for i, n := range constants.AllowedNiches {
		nicheEnum[i] = n
	}

	return map[string]interface{}{
		"type":                 "object",
		"required":             []string{"gender", "location", "niches", "subNiches", "quality"},
		"additionalProperties": false,
		"properties": map[string]interface{}{
			"gender": map[string]interface{}{
				"type":        "string",
				"description": "The deduced gender of the influencer based on Full Name, Username, and Bio/pronouns.",
				"enum":        constants.Genders,
			},
			"location": map[string]interface{}{
				"type":        "string",
				"description": "The deduced location of the influencer based on Bio and Posts' location/geo-tags.",
			},
			"niches": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "string",
					"enum": nicheEnum,
				},
				"description": "The deduced content niches from Bio, Post Content, and Hashtags. Must only contain values from the predefined enum.",
			},
			"subNiches": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "string",
				},
				"maxItems":    5,
				"description": "Optional free-form sub-niches providing finer detail beyond the broad niches. Return an empty array when the broad niches are sufficient. Typically 0-2 items, max 5.",
			},
			"quality": map[string]interface{}{
				"type":        "integer",
				"description": "Quality rating from 1-10 based on Bio quality, Post caption depth, and Engagement consistency.",
			},
		},
	}
}

// enrichmentJSONSchema is the OpenAI JSON Schema for structured outputs.
// It is strict-mode compatible (all properties required, additionalProperties false).
// The niches enum is derived from AllowedNiches automatically.
var enrichmentJSONSchema = buildEnrichmentJSONSchema()
