package gemini

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/generative-ai-go/genai"
)

type EnrichmentResult struct {
	Gender    string   `json:"gender"`
	Location  string   `json:"location"`
	Niches    []string `json:"niches"`
	SubNiches []string `json:"subNiches"`
	Quality   int      `json:"quality"`
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
		- subNiches: string array (Optional, free-form sub-niches for finer detail beyond broad niches). Rules:
		  * Not required — return an empty array if broad niches are sufficient.
		  * Be very selective — only include when it adds genuinely important specificity (e.g. "Korean Skincare" under Skincare).
		  * Maximum 5, but typically 0, 1, or 2. Prefer fewer over more.
		  * Each sub-niche should be a concise label (2-4 words max). Do NOT repeat the parent niche.
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
		
		"""{ts}
		interface EnrichmentResult {
			gender: string
			location: string
			niches: string[]
			subNiches: string[]  // optional free-form sub-niches, max 5, typically 0-2
			quality: int
		}
		"""
		
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
