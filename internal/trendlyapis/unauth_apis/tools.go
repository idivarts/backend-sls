package trendlyunauth

// ⚠️  ABUSE / COST SAFETY — READ BEFORE PRODUCTION
//
// This is an UNAUTHENTICATED, AI-cost-incurring endpoint (POST /tools/generate).
// It is called directly by the public marketing website (https://www.trendly.now)
// to power free AI generators. Each request triggers a Gemini completion, so it
// is open to abuse / cost-blowup if left uncapped.
//
// Mitigations already in place here:
//   - Input length clamped (per-field max 500 chars) and trimmed.
//   - max output tokens capped low (see maxOutputTokens below).
//   - Results capped at 5.
//
// TODO (REQUIRED before production):
//   - Add per-IP rate limiting. Easiest: API Gateway usage-plan throttling /
//     a WAF rate rule on the /tools/* path, OR a lightweight per-IP limiter
//     middleware on this group. There is no existing rate-limit middleware in
//     this repo, so it must be added.
//   - Consider a simple captcha / hCaptcha token on the website before hitting
//     this endpoint.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/generative-ai-go/genai"
	"github.com/idivarts/backend-sls/pkg/gemini"
)

const (
	maxInputLen     = 500
	maxResults      = 5
	maxOutputTokens = 600 // low cap to bound cost per call
)

// validTools maps a tool id to the required input fields it expects.
var validTools = map[string][]string{
	"instagram-caption": {"topic", "tone", "keywords"},
	"content-idea":      {"niche", "platform", "goal"},
	"hook":              {"topic", "platform"},
}

type generateToolRequest struct {
	Tool   string            `json:"tool" binding:"required"`
	Inputs map[string]string `json:"inputs" binding:"required"`
}

// GenerateToolContent powers the free AI generators on the marketing website.
// Request:  { "tool": "<id>", "inputs": { ...string fields } }
// Response: { "results": ["...", ...] }   (3-5 strings)
// Error:    HTTP 4xx/5xx with { "error": "message" }
func GenerateToolContent(c *gin.Context) {
	var req generateToolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	requiredFields, ok := validTools[req.Tool]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tool. supported: instagram-caption, content-idea, hook"})
		return
	}

	// Validate, trim and clamp inputs.
	inputs := map[string]string{}
	for _, field := range requiredFields {
		val := strings.TrimSpace(req.Inputs[field])
		if val == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("input '%s' is required", field)})
			return
		}
		if len(val) > maxInputLen {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("input '%s' exceeds %d characters", field, maxInputLen)})
			return
		}
		inputs[field] = val
	}

	if gemini.Client == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "AI service unavailable"})
		return
	}

	prompt := buildPrompt(req.Tool, inputs)

	results, err := generateStrings(prompt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate content"})
		return
	}
	if len(results) == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "no content generated"})
		return
	}
	if len(results) > maxResults {
		results = results[:maxResults]
	}

	c.JSON(http.StatusOK, gin.H{"results": results})
}

// buildPrompt returns a tool-specific instruction that asks the model to return
// a JSON array of strings.
func buildPrompt(tool string, in map[string]string) string {
	switch tool {
	case "instagram-caption":
		return fmt.Sprintf(
			"Write 3 distinct, scroll-stopping Instagram captions about \"%s\". "+
				"Tone: %s. Naturally weave in these keywords where relevant: %s. "+
				"Each caption should be 1-3 short lines and end with a few relevant hashtags. "+
				"Return ONLY a JSON array of 3 strings.",
			in["topic"], in["tone"], in["keywords"])
	case "content-idea":
		return fmt.Sprintf(
			"Generate 5 specific, actionable content ideas for the niche \"%s\" on %s, "+
				"optimized for the goal of %s. Each idea should be a concrete concept someone "+
				"could film or create today (not a vague category). "+
				"Return ONLY a JSON array of 5 strings.",
			in["niche"], in["platform"], in["goal"])
	case "hook":
		return fmt.Sprintf(
			"Write 5 scroll-stopping opening hooks for a %s post/video about \"%s\". "+
				"Each hook should be a single punchy opening line that makes viewers stop scrolling. "+
				"Return ONLY a JSON array of 5 strings.",
			in["platform"], in["topic"])
	default:
		return ""
	}
}

// generateStrings calls Gemini asking for a JSON array of strings, and parses it
// robustly — falling back to newline-splitting if JSON parsing fails.
func generateStrings(prompt string) ([]string, error) {
	model := gemini.Client.GenerativeModel("gemini-3-flash-preview")
	model.ResponseMIMEType = "application/json"
	maxTokens := int32(maxOutputTokens)
	model.MaxOutputTokens = &maxTokens

	model.SystemInstruction = genai.NewUserContent(genai.Text(
		"You are an expert social media copywriter. You always respond with ONLY a JSON array of plain strings, no extra prose.",
	))

	resp, err := model.GenerateContent(context.Background(), genai.Text(prompt))
	if err != nil {
		return nil, fmt.Errorf("failed to generate content: %w", err)
	}
	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil || len(resp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("no content returned from Gemini")
	}
	text, ok := resp.Candidates[0].Content.Parts[0].(genai.Text)
	if !ok {
		return nil, fmt.Errorf("expected text part in response")
	}

	return parseResults(string(text)), nil
}

// parseResults tries to parse a JSON array of strings; on failure it falls back
// to splitting on newlines and stripping common list prefixes.
func parseResults(raw string) []string {
	raw = strings.TrimSpace(raw)

	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err == nil {
		return cleanList(arr)
	}

	// Some models wrap the array in ```json ... ``` fences — strip and retry.
	stripped := strings.TrimSpace(strings.Trim(raw, "`"))
	stripped = strings.TrimPrefix(stripped, "json")
	if err := json.Unmarshal([]byte(strings.TrimSpace(stripped)), &arr); err == nil {
		return cleanList(arr)
	}

	// Fallback: split on newlines.
	lines := strings.Split(raw, "\n")
	return cleanList(lines)
}

// cleanList trims whitespace, strips common list markers/quotes, and drops empties.
func cleanList(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		s := strings.TrimSpace(item)
		s = strings.TrimLeft(s, "-*•0123456789. )")
		s = strings.Trim(s, `"`)
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
