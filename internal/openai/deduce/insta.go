package deduce

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/idivarts/backend-sls/pkg/myopenai"
	"github.com/openai/openai-go/v3"
)

// EnrichmentResult is the structured output returned by the LLM.
// It contains the deduced influencer attributes.
type EnrichmentResult struct {
	Gender   string   `json:"gender"`
	Location string   `json:"location"`
	Niches   []string `json:"niches"`
	Quality  int      `json:"quality"`
}

// EnrichInfluencer takes raw influencer information (bio, posts, etc.) as a
// string and returns a deduced EnrichmentResult using OpenAI structured outputs.
func EnrichInfluencer(influencerInfo string) (*EnrichmentResult, error) {
	model := "gpt-4o-2024-08-06"

	ctx := context.Background()

	schemaParam := openai.ResponseFormatJSONSchemaJSONSchemaParam{
		Name:        "enrichment_result",
		Description: openai.String("Deduced influencer enrichment data including gender, location, niches, and quality rating"),
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

// AllowedNiches is the canonical list of influencer niches.
// To add a new niche, simply append to this slice — the JSON schema and prompt
// are derived from it automatically, so no other changes are needed.
var AllowedNiches = []string{
	"Fashion",
	"Beauty",
	"Skincare",
	"Fitness",
	"Health & Wellness",
	"Yoga & Meditation",
	"Food & Cooking",
	"Travel",
	"Lifestyle",
	"Parenting & Family",
	"Home & Interior",
	"DIY & Crafts",
	"Photography",
	"Videography & Film",
	"Art & Illustration",
	"Music",
	"Dance",
	"Comedy & Entertainment",
	"Gaming",
	"Tech & Gadgets",
	"Science & Education",
	"Books & Literature",
	"Finance & Investing",
	"Business & Entrepreneurship",
	"Motivation & Self-Help",
	"Sports",
	"Outdoor & Adventure",
	"Pets & Animals",
	"Automotive",
	"Sustainability & Environment",
	"Social Causes & Activism",
	"Astrology & Spirituality",
	"Wedding & Events",
	"Luxury & High-End",
	"Plus Size & Body Positivity",
	"LGBTQ+",
	"Kids & Toys",
	"Anime & Pop Culture",
}
var Genders = []string{
	"male",
	"female",
	"couple",
	"animal",
	"lgbtq",
	"gender-neutral",
}

// enrichSystemPrompt is sent as the system message to guide the LLM.
const enrichSystemPrompt = `You are an expert Social Media Auditor. Your job is to analyze influencer data and return a JSON object.

Fields to deduce:
- gender: string (Deduce from Full Name, Username, and Bio/pronouns)
- location: string (Deduce from Bio and Posts' location/geo-tags)
- niches: string array (Deduce from Bio, Post Content, and Hashtags) — You MUST try to pick values from the predefined niche list provided in the schema enum. Choose the closest matching niche(s). If nothing fits, or you want to add a new niche, do it only when you are fully sure about the niche.
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
	nicheEnum := make([]interface{}, len(AllowedNiches))
	for i, n := range AllowedNiches {
		nicheEnum[i] = n
	}

	return map[string]interface{}{
		"type":                 "object",
		"required":             []string{"gender", "location", "niches", "quality"},
		"additionalProperties": false,
		"properties": map[string]interface{}{
			"gender": map[string]interface{}{
				"type":        "string",
				"description": "The deduced gender of the influencer based on Full Name, Username, and Bio/pronouns.",
				"enum":        Genders,
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
