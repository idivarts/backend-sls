package gemini

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/generative-ai-go/genai"
)

type EnrichmentResult struct {
	Gender   string   `json:"gender"`
	Location string   `json:"location"`
	Niches   []string `json:"niches"`
	Quality  int      `json:"quality"`
}

func EnrichInfluencer(influencerInfo string) (*EnrichmentResult, error) {
	model := Client.GenerativeModel("gemini-3-flash-preview")
	model.ResponseMIMEType = "application/json"

	model.SystemInstruction = genai.NewUserContent(genai.Text(
		`You are an expert Social Media Auditor. Your job is to analyze influencer data and return a JSON object.
		
		Fields to deduce:
		- gender: string (Deduce from Full Name, Username, and Bio/pronouns)
		- location: string (Deduce from Bio and Posts' location/geo-tags)
		- niches: string array (Deduce from Bio, Post Content, and Hashtags)
		- quality: integer (1-5) 
		  1: Poor
		  2: Average
		  3: Good
		  4: Very Good
		  5: Excellent
		  (Determine based on Bio quality, Post caption depth, and Engagement consistency)
		
		Return ONLY a JSON object matching the EnrichmentResult schema.`))

	resp, err := model.GenerateContent(context.Background(), genai.Text(influencerInfo))
	if err != nil {
		return nil, fmt.Errorf("failed to generate content: %w", err)
	}

	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil || len(resp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("no content returned from Gemini")
	}

	part := resp.Candidates[0].Content.Parts[0]
	text, ok := part.(genai.Text)
	if !ok {
		return nil, fmt.Errorf("expected text part in response")
	}

	var result EnrichmentResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal result: %w\nRaw response: %s", err, string(text))
	}

	return &result, nil
}
