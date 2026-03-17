package ai_collaboration

import (
	"context"
	"encoding/json"
	"errors"
	"log"

	"github.com/idivarts/backend-sls/pkg/myopenai"
	"github.com/openai/openai-go/v3"
)

// CollaborationPromptResponse is the structured output returned by the LLM.
// It either contains an error (not enough info) or a collaboration draft.
type CollaborationPromptResponse struct {
	Error         bool                `json:"error"`
	ErrorMessage  *string             `json:"errorMessage"`
	Collaboration *CollaborationDraft `json:"collaboration"`
}

// CollaborationDraft is the subset of Collaboration fields that can be
// generated from a user prompt. System-managed fields (brandId, managerId,
// status, timestamps, etc.) are intentionally excluded.
type CollaborationDraft struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	PromotionType string `json:"promotionType"`

	Budget *struct {
		Min *int `json:"min"`
		Max *int `json:"max"`
	} `json:"budget"`

	PreferredContentLanguage  []string `json:"preferredContentLanguage"`
	ContentFormat             []string `json:"contentFormat"`
	Platform                  []string `json:"platform"`
	NumberOfInfluencersNeeded int      `json:"numberOfInfluencersNeeded"`
	QuestionsToInfluencers    []string `json:"questionsToInfluencers"`
	RelevantImages            []string `json:"relevantImages"`
	ExternalLinks             []struct {
		Name string `json:"name"`
		Link string `json:"link"`
	} `json:"externalLinks"`
}

func (CollaborationDraft) GetResults(prompt string, brandDetails string) (*CollaborationDraft, error) {
	model := "gpt-4o-2024-08-06"

	ctx := context.Background()

	schemaParam := openai.ResponseFormatJSONSchemaJSONSchemaParam{
		Name:        "collaboration_response",
		Description: openai.String("Generate a collaboration draft or return an error if not enough information"),
		Schema:      collabPromptJSONSchema,
		Strict:      openai.Bool(true),
	}

	userPrompt := prompt
	if brandDetails != "" {
		userPrompt = prompt + "\n\nAdditional brand details:\n```{json}" + brandDetails + "```"
	}

	chat, err := myopenai.Client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(collabSystemPrompt),
			openai.UserMessage(userPrompt),
		},
		Model: openai.ChatModel(model),
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
				JSONSchema: schemaParam,
			},
		},
		WebSearchOptions: openai.ChatCompletionNewParamsWebSearchOptions{
			UserLocation: openai.ChatCompletionNewParamsWebSearchOptionsUserLocation{
				Approximate: openai.ChatCompletionNewParamsWebSearchOptionsUserLocationApproximate{
					Country: openai.String("IN"),
				},
			},
		},
	})

	if err != nil {
		log.Println("Error generating collaboration from prompt:", err.Error())
		return nil, err
	}

	if len(chat.Choices) == 0 {
		return nil, errors.New("no response from model")
	}

	rawJSON := chat.Choices[0].Message.Content

	var result *CollaborationPromptResponse
	if err := json.Unmarshal([]byte(rawJSON), &result); err != nil {
		log.Println("Error parsing structured response:", err.Error())
		return nil, err
	}

	if result.Error {
		return nil, errors.New(*result.ErrorMessage)
	}

	return result.Collaboration, nil
}

// collabSystemPrompt is sent as the system message to guide the LLM.
const collabSystemPrompt = `You are an AI assistant that helps create influencer marketing collaboration campaigns on a platform called Trendly.

Based on the user's description, generate a structured collaboration draft that gives them a solid starting point they can refine.

Rules:
1. If the prompt does NOT contain enough information to create a meaningful collaboration draft — at a minimum it should convey what the collaboration/campaign is about — set "error" to true and provide a helpful "errorMessage" explaining what is missing.
2. If there IS enough information, set "error" to false, "errorMessage" to null, and populate the "collaboration" object as completely as possible:
   - "name": A catchy, descriptive campaign title.
   - "description": A detailed description including deliverables, expectations, and key details.
   - "promotionType": One of "sponsored-post", "product-review", "giveaway", "brand-ambassador", "affiliate", "event", or "other".
   - "budget": If the user mentions a budget, populate min/max (integers). Otherwise set to null.
   - "preferredContentLanguage": Infer language(s) from context (default to ["English"] if unclear).
   - "contentFormat": Appropriate content types, e.g. ["reel", "story", "post", "video", "short"].
   - "platform": Target social media platforms, e.g. ["instagram", "youtube", "tiktok"].
   - "numberOfInfluencersNeeded": Use explicit number if given, otherwise pick a reasonable default (e.g. 5).
   - "questionsToInfluencers": Generate 2-3 relevant screening questions for applicants based on the collaboration context.
   - "relevantImages": Array of image URLs relevant to the campaign/brand. Max 6 items.
   - "externalLinks": Array of quick links for the listing. Max 2 items. Each item must be {"name": "...", "link": "..."}.
3. When the user's prompt is vague on certain fields, make smart defaults rather than leaving them empty. The goal is to give the user a usable draft.
4. If "Additional brand details" are provided in the user message, treat them as high-confidence context and use them to refine campaign name, description, targeting, and screening questions.
5. If brand details contain a website URL, use web search/browsing context to identify up to 6 relevant image URLs from that website for "relevantImages". If no website is available, return [].
6. "externalLinks" should include up to 2 useful links users can open quickly (for example official website, key product/category page, social page). If no reliable links are available, return [].
7. Never invent broken-looking URLs. Prefer canonical public URLs and keep output JSON schema-compliant.`

// collabPromptJSONSchema is the OpenAI JSON Schema for structured outputs.
// It is strict-mode compatible (all properties required, additionalProperties false,
// nullable fields use anyOf with null).
var collabPromptJSONSchema = map[string]interface{}{
	"type":                 "object",
	"required":             []string{"error", "errorMessage", "collaboration"},
	"additionalProperties": false,
	"properties": map[string]interface{}{
		"error": map[string]interface{}{
			"type":        "boolean",
			"description": "True if the prompt does not have enough information to generate a collaboration draft.",
		},
		"errorMessage": map[string]interface{}{
			"anyOf": []interface{}{
				map[string]interface{}{"type": "string"},
				map[string]interface{}{"type": "null"},
			},
			"description": "A helpful message explaining what information is missing. Null when error is false.",
		},
		"collaboration": map[string]interface{}{
			"anyOf": []interface{}{
				map[string]interface{}{
					"type":                 "object",
					"required":             []string{"name", "description", "promotionType", "budget", "preferredContentLanguage", "contentFormat", "platform", "numberOfInfluencersNeeded", "questionsToInfluencers", "relevantImages", "externalLinks"},
					"additionalProperties": false,
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type":        "string",
							"description": "A catchy, descriptive campaign name for the collaboration.",
						},
						"description": map[string]interface{}{
							"type":        "string",
							"description": "A detailed description of the collaboration including deliverables, expectations, and any special requirements.",
						},
						"promotionType": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"sponsored-post", "product-review", "giveaway", "brand-ambassador", "affiliate", "event", "other"},
							"description": "The type of promotion for this collaboration.",
						},
						"budget": map[string]interface{}{
							"anyOf": []interface{}{
								map[string]interface{}{
									"type":                 "object",
									"required":             []string{"min", "max"},
									"additionalProperties": false,
									"properties": map[string]interface{}{
										"min": map[string]interface{}{
											"anyOf": []interface{}{
												map[string]interface{}{"type": "integer"},
												map[string]interface{}{"type": "null"},
											},
											"description": "Minimum budget amount.",
										},
										"max": map[string]interface{}{
											"anyOf": []interface{}{
												map[string]interface{}{"type": "integer"},
												map[string]interface{}{"type": "null"},
											},
											"description": "Maximum budget amount.",
										},
									},
								},
								map[string]interface{}{"type": "null"},
							},
							"description": "Budget range for the collaboration. Null if not specified or barter-based.",
						},
						"preferredContentLanguage": map[string]interface{}{
							"type":        "array",
							"items":       map[string]interface{}{"type": "string"},
							"description": "Preferred languages for the content, e.g. ['English', 'Hindi'].",
						},
						"contentFormat": map[string]interface{}{
							"type":        "array",
							"items":       map[string]interface{}{"type": "string"},
							"description": "Content types expected, e.g. ['reel', 'story', 'post', 'video', 'short'].",
						},
						"platform": map[string]interface{}{
							"type":        "array",
							"items":       map[string]interface{}{"type": "string"},
							"description": "Target social media platforms, e.g. ['instagram', 'youtube', 'tiktok'].",
						},
						"numberOfInfluencersNeeded": map[string]interface{}{
							"type":        "integer",
							"description": "Number of influencers needed. Default to a reasonable number if not explicitly stated.",
						},
						"questionsToInfluencers": map[string]interface{}{
							"type":        "array",
							"items":       map[string]interface{}{"type": "string"},
							"description": "Screening questions to ask influencers when they apply. Generate 2-3 relevant questions.",
						},
						"relevantImages": map[string]interface{}{
							"type":        "array",
							"maxItems":    6,
							"items":       map[string]interface{}{"type": "string"},
							"description": "Relevant image URLs for the campaign. Return [] if not available.",
						},
						"externalLinks": map[string]interface{}{
							"type":        "array",
							"maxItems":    2,
							"description": "Quick links relevant to the campaign/brand. Return [] if not available.",
							"items": map[string]interface{}{
								"type":                 "object",
								"required":             []string{"name", "link"},
								"additionalProperties": false,
								"properties": map[string]interface{}{
									"name": map[string]interface{}{
										"type":        "string",
										"description": "Short display name for quick link.",
									},
									"link": map[string]interface{}{
										"type":        "string",
										"description": "Public URL for the quick link.",
									},
								},
							},
						},
					},
				},
				map[string]interface{}{"type": "null"},
			},
			"description": "The generated collaboration draft. Null when error is true.",
		},
	},
}
