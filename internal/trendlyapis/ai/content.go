package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// ───────────────────────── Caption (sync HTTP) ─────────────────────────

type captionReq struct {
	BrandID   string `json:"brandId" binding:"required"`
	Topic     string `json:"topic" binding:"required"`
	Platform  string `json:"platform"`
	Format    string `json:"format"`
	Tone      string `json:"tone"`
	ContextID string `json:"contextId"`
	Model     string `json:"model"`

	// Live editor content — the current, possibly-unsaved state of the piece the
	// user is working on. When present it overrides the persisted-doc context so
	// the AI writes with what's on screen now.
	Title       string `json:"title"`
	Description string `json:"description"`
	Caption     string `json:"caption"`
	Hashtags    string `json:"hashtags"`
	Script      string `json:"script"`
}

type captionVariant struct {
	Length string `json:"length"`
	Text   string `json:"text"`
}

func HTTPCaption(c *gin.Context) {
	managerID, _ := middlewares.GetUserId(c)
	var req captionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !verifyBrandAccess(req.BrandID, managerID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	brand, err := loadBrand(req.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "brand not found"})
		return
	}

	liveBrief := briefFromFields(req.Title, req.Platform, req.Format, req.Description, req.Caption, req.Hashtags, req.Script)
	sys := systemPromptWithLiveContent(brand, req.BrandID, req.ContextID, liveBrief)
	model, locked := pickModel(c.Request.Context(), req.BrandID, openrouter.TaskCaption, req.Model)
	if locked {
		c.JSON(http.StatusPaymentRequired, gin.H{"error": "upgrade_required", "task": openrouter.TaskCaption})
		return
	}
	orgID, _ := orgIDForBrand(req.BrandID)
	if aiTokensExhausted(orgID) {
		c.JSON(http.StatusPaymentRequired, gin.H{"error": "upgrade_required", "reason": "tokens_exhausted", "task": openrouter.TaskCaption})
		return
	}

	user := fmt.Sprintf(
		"Write 3 caption options for a %s %s post. Topic: %s. Tone: %s.\n\nReturn STRICTLY a JSON array of 3 objects with keys \"length\" (\"short\"|\"medium\"|\"long\") and \"text\". No markdown, no commentary.",
		req.Platform, req.Format, req.Topic, req.Tone,
	)

	resp, err := openrouter.ChatCompletion(c.Request.Context(), openrouter.ChatRequest{
		Model:    model,
		Messages: []openrouter.Message{{Role: "system", Content: sys}, {Role: "user", Content: user}},
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	meterAIUsage(orgID, resp.Usage)

	raw := ""
	if len(resp.Choices) > 0 {
		raw = resp.Choices[0].Message.Content
	}
	variants := parseCaptionJSON(raw)

	c.JSON(http.StatusOK, gin.H{
		"variants": variants,
		"model":    model,
		"usage":    resp.Usage,
	})
}

var jsonArrayRe = regexp.MustCompile(`(?s)\[.*\]`)

func parseCaptionJSON(raw string) []captionVariant {
	raw = strings.TrimSpace(raw)
	match := jsonArrayRe.FindString(raw)
	if match == "" {
		return []captionVariant{{Length: "medium", Text: raw}}
	}
	var out []captionVariant
	if err := json.Unmarshal([]byte(match), &out); err != nil {
		return []captionVariant{{Length: "medium", Text: raw}}
	}
	return out
}

// ───────────────────────── Hashtags (sync HTTP) ─────────────────────────

type hashtagReq struct {
	BrandID   string `json:"brandId" binding:"required"`
	Topic     string `json:"topic" binding:"required"`
	Platform  string `json:"platform"`
	ContextID string `json:"contextId"`
	Model     string `json:"model"`

	// Live editor content (see captionReq).
	Title       string `json:"title"`
	Format      string `json:"format"`
	Description string `json:"description"`
	Caption     string `json:"caption"`
	Hashtags    string `json:"hashtags"`
	Script      string `json:"script"`
}

type hashtagGroup struct {
	Tier string   `json:"tier"`
	Tags []string `json:"tags"`
}

func HTTPHashtags(c *gin.Context) {
	managerID, _ := middlewares.GetUserId(c)
	var req hashtagReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !verifyBrandAccess(req.BrandID, managerID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	brand, err := loadBrand(req.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "brand not found"})
		return
	}

	liveBrief := briefFromFields(req.Title, req.Platform, req.Format, req.Description, req.Caption, req.Hashtags, req.Script)
	sys := systemPromptWithLiveContent(brand, req.BrandID, req.ContextID, liveBrief)
	model, locked := pickModel(c.Request.Context(), req.BrandID, openrouter.TaskHashtag, req.Model)
	if locked {
		c.JSON(http.StatusPaymentRequired, gin.H{"error": "upgrade_required", "task": openrouter.TaskHashtag})
		return
	}
	orgID, _ := orgIDForBrand(req.BrandID)
	if aiTokensExhausted(orgID) {
		c.JSON(http.StatusPaymentRequired, gin.H{"error": "upgrade_required", "reason": "tokens_exhausted", "task": openrouter.TaskHashtag})
		return
	}

	user := fmt.Sprintf(
		"Suggest hashtags for a %s post on the topic: %s. Use up-to-date trending hashtags when possible.\n\nReturn STRICTLY a JSON array of 3 objects with keys \"tier\" (\"broad\"|\"niche\"|\"brand\") and \"tags\" (array of strings, no # prefix). 5-7 tags per tier. No markdown, no commentary.",
		req.Platform, req.Topic,
	)

	resp, err := openrouter.ChatCompletion(c.Request.Context(), openrouter.ChatRequest{
		Model:    model,
		Messages: []openrouter.Message{{Role: "system", Content: sys}, {Role: "user", Content: user}},
		Plugins:  []openrouter.Plugin{openrouter.WebSearchPlugin()},
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	meterAIUsage(orgID, resp.Usage)
	raw := ""
	if len(resp.Choices) > 0 {
		raw = resp.Choices[0].Message.Content
	}
	groups := parseHashtagJSON(raw)
	c.JSON(http.StatusOK, gin.H{
		"groups": groups,
		"model":  model,
		"usage":  resp.Usage,
	})
}

func parseHashtagJSON(raw string) []hashtagGroup {
	raw = strings.TrimSpace(raw)
	match := jsonArrayRe.FindString(raw)
	if match == "" {
		return nil
	}
	var out []hashtagGroup
	if err := json.Unmarshal([]byte(match), &out); err != nil {
		return nil
	}
	return out
}

// ───────────────────────── Script (streamed via WS) ─────────────────────────

type scriptPayload struct {
	BrandID    string `json:"brandId"`
	ContextID  string `json:"contextId"`
	VideoType  string `json:"videoType"`
	Topic      string `json:"topic"`
	KeyMessage string `json:"keyMessage"`
	Tone       string `json:"tone"`

	// Live editor content (see captionReq).
	Title       string `json:"title"`
	Platform    string `json:"platform"`
	Format      string `json:"format"`
	Description string `json:"description"`
	Caption     string `json:"caption"`
	Hashtags    string `json:"hashtags"`
	Script      string `json:"script"`
}

// systemPromptWithLiveContent builds the content-module system prompt, preferring
// the live (possibly unsaved) editor brief over the last-saved Firestore doc.
// When a live brief is supplied we suppress the persisted-doc load (contextID)
// so the AI doesn't get two conflicting versions of the same piece.
func systemPromptWithLiveContent(brand *trendlymodels.Brand, brandID, contextID, liveBrief string) string {
	ctxID := contextID
	if liveBrief != "" {
		ctxID = ""
	}
	sys := buildSystemPrompt(brand, "content", brandID, ctxID, "")
	if liveBrief != "" {
		sys += "\nThe content the user is working on right now (may include unsaved edits — treat this as the current state of the piece):\n" + liveBrief + "\n"
	}
	return sys
}

func handleScriptGenWS(req WSRequest) {
	var p scriptPayload
	if err := decodePayload(req.Payload, &p); err != nil {
		wsErrorTo(req.ConnectionID, "invalid payload: "+err.Error())
		return
	}
	if req.BrandID != "" {
		p.BrandID = req.BrandID
	}
	if !verifyBrandAccess(p.BrandID, req.UserID) {
		wsErrorTo(req.ConnectionID, "forbidden")
		return
	}
	brand, err := loadBrand(p.BrandID)
	if err != nil {
		wsErrorTo(req.ConnectionID, "brand not found")
		return
	}
	liveBrief := briefFromFields(p.Title, p.Platform, p.Format, p.Description, p.Caption, p.Hashtags, p.Script)
	sys := systemPromptWithLiveContent(brand, p.BrandID, p.ContextID, liveBrief)
	model, locked := pickModel(context.Background(), p.BrandID, openrouter.TaskScript, req.Model)
	if locked {
		wsSend(req.ConnectionID, map[string]any{"type": "upgrade_required", "task": string(openrouter.TaskScript)})
		return
	}
	orgID, _ := orgIDForBrand(p.BrandID)
	if aiTokensExhausted(orgID) {
		wsSend(req.ConnectionID, map[string]any{"type": "upgrade_required", "reason": "tokens_exhausted", "task": string(openrouter.TaskScript)})
		return
	}

	user := fmt.Sprintf(
		"Write a video script for a %s. Topic: %s. Key message: %s. Tone: %s.\n\nReturn structured markdown with these sections:\n## Hook (first 3 seconds)\n## Body (3-4 scenes with cues)\n## CTA",
		p.VideoType, p.Topic, p.KeyMessage, p.Tone,
	)

	// Accumulate the streamed script so it can be persisted on completion — this
	// makes the result survive a websocket drop and keeps the content doc (and
	// thus the AI chat context) up to date.
	var scriptBuf strings.Builder
	streamErr := openrouter.ChatCompletionStream(context.Background(), openrouter.ChatRequest{
		Model:    model,
		Messages: []openrouter.Message{{Role: "system", Content: sys}, {Role: "user", Content: user}},
	}, openrouter.StreamCallbacks{
		OnDelta: func(delta string) {
			scriptBuf.WriteString(delta)
			wsSend(req.ConnectionID, map[string]any{"type": "token", "delta": delta})
		},
		OnDone: func(u *openrouter.Usage) {
			if p.ContextID != "" {
				if s := strings.TrimSpace(scriptBuf.String()); s != "" {
					if err := trendlymodels.UpdateContentFields(p.BrandID, p.ContextID, map[string]any{"script": s}); err != nil {
						log.Printf("ai script persist: %v", err)
					}
				}
			}
			meterAIUsage(orgID, u)
			wsSend(req.ConnectionID, map[string]any{"type": "done", "usage": u})
		},
		OnError: func(e error) {
			wsErrorTo(req.ConnectionID, e.Error())
		},
	})
	if streamErr != nil {
		log.Printf("ai script stream: %v", streamErr)
	}
}

// ───────────────────────── Image (single + carousel, async via WS) ─────────────────────────

// moduleMedia is the dedicated AI thread for a content's image generate/enhance
// iterations (one conversation per content, contextId = contentId). Isolated
// from the content text chat (module="content").
const moduleMedia = "media"

type imagePayload struct {
	BrandID     string `json:"brandId"`
	ContextID   string `json:"contextId"` // content doc id — where results + status are persisted
	Description string `json:"description"`
	Style       string `json:"style"`
	AspectRatio string `json:"aspectRatio"`
	Count       int    `json:"count"`
	Multi       bool   `json:"multi"` // carousel: append; single: replace the image
	// FocusedSlideIndex is the carousel slide the user focused (0-based), if any.
	// Used as the edit target / base slide. nil → no focus (implicit 0 for single).
	FocusedSlideIndex *int `json:"focusedSlideIndex"`
}

// handleImageGenWS runs a stateful media generation turn. The first time (no
// image and no media thread) it does text-to-image ("Generate"); once an image
// exists (AI-generated OR user-uploaded) it does image-to-image with the media
// thread's context ("Enhance"). For carousels the model decides edit-focused-slide
// vs add-new-slide. Disconnect-proof: status + results persist on the content doc.
func handleImageGenWS(req WSRequest) {
	var p imagePayload
	if err := decodePayload(req.Payload, &p); err != nil {
		wsErrorTo(req.ConnectionID, "invalid payload: "+err.Error())
		return
	}
	if req.BrandID != "" {
		p.BrandID = req.BrandID
	}
	if !verifyBrandAccess(p.BrandID, req.UserID) {
		wsErrorTo(req.ConnectionID, "forbidden")
		return
	}
	if p.Count <= 0 {
		p.Count = 1
	}
	if p.Count > 10 {
		p.Count = 10
	}

	model, locked := pickModel(context.Background(), p.BrandID, openrouter.TaskImage, req.Model)
	if locked {
		wsSend(req.ConnectionID, map[string]any{"type": "upgrade_required", "task": string(openrouter.TaskImage)})
		return
	}
	orgID, _ := orgIDForBrand(p.BrandID)
	if aiTokensExhausted(orgID) {
		wsSend(req.ConnectionID, map[string]any{"type": "upgrade_required", "reason": "tokens_exhausted", "task": string(openrouter.TaskImage)})
		return
	}

	// Load the content doc — its attachments are the source of truth for base
	// images, and it stores the media-thread id.
	var content *trendlymodels.Content
	if p.ContextID != "" {
		if ct, err := trendlymodels.GetContent(p.BrandID, p.ContextID); err == nil {
			content = ct
		}
	}
	var existing []trendlymodels.ContentAttachment
	if content != nil {
		existing = content.Attachments
	}
	baseImages := imageAttachmentURLs(existing)

	// Generate (no image and no prior thread) vs Enhance (image exists or thread
	// exists). Uploaded bases count as "image exists" → enhance.
	mediaConvID := ""
	if content != nil {
		mediaConvID = content.MediaConversationID
	}
	isEnhance := len(baseImages) > 0 || mediaConvID != ""

	// Get or create the dedicated media thread, then append the user's prompt.
	convID, history := ensureMediaConversation(p.BrandID, req.UserID, p.ContextID, content, model)
	if convID != "" {
		_, _ = openrouter.AppendMessage(context.Background(), convID, trendlymodels.AIMessage{
			Role: "user", UserID: req.UserID, BrandID: p.BrandID,
			Content: p.Description, Timestamp: time.Now().UnixMilli(),
		})
	}

	setImageGenStatus(p.BrandID, p.ContextID, map[string]any{
		"status":         "generating",
		"prompt":         p.Description,
		"error":          "",
		"requestedCount": p.Count,
		"completedCount": 0,
		"startedAt":      time.Now().UnixMilli(),
	})

	if isEnhance {
		runMediaEnhance(req, p, model, orgID, convID, existing, baseImages, history)
		return
	}
	runMediaGenerate(req, p, model, orgID, convID, existing)
}

// runMediaGenerate is the first-time, text-to-image path. Single types replace
// the image; carousels append each generated slide. Persisted incrementally so
// the result survives a socket drop.
func runMediaGenerate(req WSRequest, p imagePayload, model, orgID, convID string, existing []trendlymodels.ContentAttachment) {
	size := aspectToSize(p.AspectRatio)
	prompt := p.Description
	if p.Style != "" {
		prompt = fmt.Sprintf("%s. Style: %s.", p.Description, p.Style)
	}

	generated := make([]trendlymodels.ContentAttachment, 0, p.Count)
	completed := 0
	var firstErr error
	var imgCost float64
	for i := 0; i < p.Count; i++ {
		wsSend(req.ConnectionID, map[string]any{"type": "image_status", "index": i, "state": "generating"})
		resp, usage, err := openrouter.GenerateImage(context.Background(), openrouter.ImageRequest{
			Model: model, Prompt: prompt, Size: size, N: 1,
		})
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			wsErrorTo(req.ConnectionID, fmt.Sprintf("image %d failed: %v", i, err))
			continue
		}
		if usage != nil {
			imgCost += usage.Cost
		}
		url, uerr := uploadFirstImage(resp, p.BrandID)
		if url == "" {
			if firstErr == nil {
				firstErr = uerr
			}
			wsErrorTo(req.ConnectionID, fmt.Sprintf("image %d: upload failed", i))
			continue
		}
		generated = append(generated, trendlymodels.ContentAttachment{Type: "image", ImageURL: url})
		completed++
		persistGeneratedImages(p.BrandID, p.ContextID, existing, generated, p.Multi)
		setImageGenStatus(p.BrandID, p.ContextID, map[string]any{"status": "generating", "completedCount": completed})
		wsSend(req.ConnectionID, map[string]any{"type": "image", "index": i, "s3Url": url})
	}

	meterAIUsage(orgID, &openrouter.Usage{Cost: imgCost})
	finishMediaTurn(req, p, convID, completed, firstErr, generated, "Generated")
}

// runMediaEnhance is the image-to-image iteration path. It uses the current
// image(s) as visual input plus the thread's textual history, and for carousels
// decides between editing the focused slide (replace at index) and adding a new
// slide (append). Single types always edit/replace.
func runMediaEnhance(req WSRequest, p imagePayload, model, orgID, convID string, existing []trendlymodels.ContentAttachment, baseImages []string, history []trendlymodels.AIMessage) {
	size := aspectToSize(p.AspectRatio)

	// Provenance: a user-uploaded base has no prior assistant image message.
	provenance := "The current image was generated by AI in this thread."
	if !historyHasAssistantImage(history) && len(baseImages) > 0 {
		provenance = "The current image was uploaded by the user (not AI-generated). Treat it as the base to edit."
	}

	// Decide edit vs add (carousel) and the target slide.
	action := "edit"
	targetIndex := 0
	if p.FocusedSlideIndex != nil {
		targetIndex = *p.FocusedSlideIndex
	}
	var inputImages []string
	if p.Multi {
		action, targetIndex = classifyCarouselIntent(model, p.Description, p.FocusedSlideIndex, len(baseImages))
		if action == "add" {
			// Keep all existing slides as visual context for a style-consistent new slide.
			inputImages = baseImages
		} else {
			if targetIndex < 0 || targetIndex >= len(baseImages) {
				targetIndex = 0
			}
			inputImages = []string{baseImages[targetIndex]}
		}
	} else {
		// Single post/story: edit/replace the one image.
		action = "edit"
		targetIndex = 0
		if len(baseImages) > 0 {
			inputImages = []string{baseImages[0]}
		}
	}

	prompt := buildEnhancePrompt(p.Description, p.Style, provenance, history)

	wsSend(req.ConnectionID, map[string]any{"type": "image_status", "index": targetIndex, "state": "generating"})
	resp, usage, err := openrouter.GenerateImage(context.Background(), openrouter.ImageRequest{
		Model: model, Prompt: prompt, Size: size, N: 1, InputImages: inputImages,
	})
	if err != nil {
		meterAIUsage(orgID, nil)
		finishMediaTurn(req, p, convID, 0, err, nil, "Enhanced")
		wsErrorTo(req.ConnectionID, "enhance failed: "+err.Error())
		return
	}
	if usage != nil {
		meterAIUsage(orgID, &openrouter.Usage{Cost: usage.Cost})
	}
	url, uerr := uploadFirstImage(resp, p.BrandID)
	if url == "" {
		finishMediaTurn(req, p, convID, 0, uerr, nil, "Enhanced")
		wsErrorTo(req.ConnectionID, "enhance: upload failed")
		return
	}

	att := trendlymodels.ContentAttachment{Type: "image", ImageURL: url}
	var next []trendlymodels.ContentAttachment
	if p.Multi && action == "add" {
		next = append(append([]trendlymodels.ContentAttachment{}, existing...), att)
	} else if p.Multi {
		next = replaceAttachmentAt(existing, targetIndex, att)
	} else {
		next = []trendlymodels.ContentAttachment{att}
	}
	if p.ContextID != "" {
		if err := trendlymodels.UpdateContentFields(p.BrandID, p.ContextID, map[string]any{"attachments": next}); err != nil {
			log.Printf("ai media enhance persist: %v", err)
		}
	}
	wsSend(req.ConnectionID, map[string]any{"type": "image", "index": targetIndex, "s3Url": url})

	note := "Enhanced"
	if p.Multi && action == "add" {
		note = "Added a new slide"
	} else if p.Multi {
		note = fmt.Sprintf("Edited slide %d", targetIndex+1)
	}
	finishMediaTurn(req, p, convID, 1, nil, []trendlymodels.ContentAttachment{att}, note)
}

// finishMediaTurn writes the terminal status, appends the assistant message to
// the media thread (with the resulting image URLs), and signals done.
func finishMediaTurn(req WSRequest, p imagePayload, convID string, completed int, firstErr error, generated []trendlymodels.ContentAttachment, verb string) {
	if completed == 0 {
		msg := "image generation failed"
		if firstErr != nil {
			msg = firstErr.Error()
		}
		setImageGenStatus(p.BrandID, p.ContextID, map[string]any{"status": "error", "error": msg})
		wsSend(req.ConnectionID, map[string]any{"type": "done"})
		return
	}
	setImageGenStatus(p.BrandID, p.ContextID, map[string]any{"status": "done", "error": "", "completedCount": completed})

	if convID != "" {
		urls := imageAttachmentURLs(generated)
		_, _ = openrouter.AppendMessage(context.Background(), convID, trendlymodels.AIMessage{
			Role: "assistant", UserID: req.UserID, BrandID: p.BrandID,
			Content: fmt.Sprintf("%s %d image(s).", verb, completed),
			Images:  urls, Timestamp: time.Now().UnixMilli(),
		})
	}
	wsSend(req.ConnectionID, map[string]any{"type": "done"})
}

// ensureMediaConversation returns the content's media thread id (creating it and
// stamping it on the content doc on first use) plus its message history. Returns
// "" when there is no content context (ad-hoc generation not tied to a piece).
func ensureMediaConversation(brandID, userID, contentID string, content *trendlymodels.Content, model string) (string, []trendlymodels.AIMessage) {
	if contentID == "" {
		return "", nil
	}
	ctx := context.Background()
	if content != nil && content.MediaConversationID != "" {
		hist, _ := openrouter.LoadHistory(ctx, content.MediaConversationID)
		return content.MediaConversationID, hist
	}
	title := "Media"
	if content != nil && content.Title != "" {
		title = content.Title
	}
	conv, err := openrouter.CreateConversation(ctx, brandID, userID, moduleMedia, contentID, model, title)
	if err != nil || conv == nil {
		log.Printf("ai media conversation create: %v", err)
		return "", nil
	}
	if err := trendlymodels.UpdateContentFields(brandID, contentID, map[string]any{
		"mediaConversationId": conv.ID,
	}); err != nil {
		log.Printf("ai media conversation stamp: %v", err)
	}
	return conv.ID, nil
}

// classifyCarouselIntent asks the model whether the prompt wants to edit an
// existing slide or add a new one, and which slide to target. Best-effort: on
// any failure it falls back to edit-the-focused-slide (or add when unfocused).
func classifyCarouselIntent(_ string, prompt string, focused *int, slideCount int) (action string, targetIndex int) {
	fallbackAction := "add"
	fallbackIndex := 0
	if focused != nil {
		fallbackAction = "edit"
		fallbackIndex = *focused
	}

	classModel, locked := pickModel(context.Background(), "", openrouter.TaskChat, "")
	if locked || classModel == "" {
		return fallbackAction, fallbackIndex
	}
	focusDesc := "no specific slide"
	if focused != nil {
		focusDesc = fmt.Sprintf("slide index %d", *focused)
	}
	sys := "You classify a carousel image instruction. Respond with ONLY JSON, no prose."
	user := fmt.Sprintf(
		"The carousel has %d slide(s), 0-indexed. The user focused: %s. Instruction: %q.\n"+
			"Decide if they want to EDIT an existing slide or ADD a new slide. "+
			"Return JSON {\"action\":\"edit\"|\"add\",\"targetIndex\":<int 0-based slide to edit, or the focused slide>}.",
		slideCount, focusDesc, prompt,
	)
	resp, err := openrouter.ChatCompletion(context.Background(), openrouter.ChatRequest{
		Model:          classModel,
		Messages:       []openrouter.Message{{Role: "system", Content: sys}, {Role: "user", Content: user}},
		ResponseFormat: &openrouter.ResponseFormat{Type: "json_object"},
	})
	if err != nil || len(resp.Choices) == 0 {
		return fallbackAction, fallbackIndex
	}
	var parsed struct {
		Action      string `json:"action"`
		TargetIndex int    `json:"targetIndex"`
	}
	raw := jsonObjectRe.FindString(resp.Choices[0].Message.Content)
	if raw == "" || json.Unmarshal([]byte(raw), &parsed) != nil {
		return fallbackAction, fallbackIndex
	}
	if parsed.Action != "edit" && parsed.Action != "add" {
		return fallbackAction, fallbackIndex
	}
	return parsed.Action, parsed.TargetIndex
}

var jsonObjectRe = regexp.MustCompile(`(?s)\{.*\}`)

// buildEnhancePrompt folds the provenance note, a short thread-history summary
// and the user's edit instruction into a single image prompt. The base image(s)
// are passed separately as InputImages (the visual channel).
func buildEnhancePrompt(userPrompt, style, provenance string, history []trendlymodels.AIMessage) string {
	var sb strings.Builder
	sb.WriteString(provenance)
	if h := summariseMediaHistory(history); h != "" {
		sb.WriteString("\n\nEarlier in this image thread:\n")
		sb.WriteString(h)
	}
	sb.WriteString("\n\nApply this change to the provided image, keeping everything else consistent: ")
	sb.WriteString(userPrompt)
	if style != "" {
		sb.WriteString(fmt.Sprintf(" (Style: %s.)", style))
	}
	return sb.String()
}

// summariseMediaHistory renders the last few thread turns into a compact textual
// recap so the image model has the iteration context.
func summariseMediaHistory(history []trendlymodels.AIMessage) string {
	if len(history) == 0 {
		return ""
	}
	start := 0
	if len(history) > 6 {
		start = len(history) - 6
	}
	var lines []string
	for _, m := range history[start:] {
		txt := strings.TrimSpace(m.Content)
		if txt == "" {
			continue
		}
		who := "User"
		if m.Role == "assistant" {
			who = "AI"
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", who, txt))
	}
	return strings.Join(lines, "\n")
}

// historyHasAssistantImage reports whether the thread already contains an
// AI-generated image (used to tell uploaded vs generated bases apart).
func historyHasAssistantImage(history []trendlymodels.AIMessage) bool {
	for _, m := range history {
		if m.Role == "assistant" && len(m.Images) > 0 {
			return true
		}
	}
	return false
}

// imageAttachmentURLs returns the image URLs of the given attachments, in order.
func imageAttachmentURLs(atts []trendlymodels.ContentAttachment) []string {
	var out []string
	for _, a := range atts {
		if a.Type == "image" && a.ImageURL != "" {
			out = append(out, a.ImageURL)
		}
	}
	return out
}

// replaceAttachmentAt returns existing with index idx replaced by att (appended
// when idx is out of range).
func replaceAttachmentAt(existing []trendlymodels.ContentAttachment, idx int, att trendlymodels.ContentAttachment) []trendlymodels.ContentAttachment {
	next := append([]trendlymodels.ContentAttachment{}, existing...)
	if idx >= 0 && idx < len(next) {
		next[idx] = att
		return next
	}
	return append(next, att)
}

// uploadFirstImage uploads the first image off a GenerateImage response to S3 and
// returns its CloudFront URL (hosted URL or base64, whichever the model returned).
func uploadFirstImage(resp *openrouter.ImageResponse, brandID string) (string, error) {
	if resp == nil || len(resp.Data) == 0 {
		return "", fmt.Errorf("no image returned")
	}
	d := resp.Data[0]
	if d.URL != "" {
		return uploadFromURL(d.URL, brandID)
	}
	if d.B64JSON != "" {
		return uploadBase64Image(d.B64JSON, brandID)
	}
	return "", fmt.Errorf("empty image data")
}

// setImageGenStatus merge-updates the imageGeneration block on the content doc.
// No-ops when there is no content context (ad-hoc generation not tied to a piece).
func setImageGenStatus(brandID, contentID string, fields map[string]any) {
	if contentID == "" {
		return
	}
	fields["updatedAt"] = time.Now().UnixMilli()
	if err := trendlymodels.UpdateContentFields(brandID, contentID, map[string]any{
		"imageGeneration": fields,
	}); err != nil {
		log.Printf("ai image-gen status update: %v", err)
	}
}

// persistGeneratedImages writes the content doc's attachments so generated
// images are saved without the user pressing Save. Carousels append to the
// existing set; single-image types replace it.
func persistGeneratedImages(brandID, contentID string, existing, generated []trendlymodels.ContentAttachment, multi bool) {
	if contentID == "" {
		return
	}
	var next []trendlymodels.ContentAttachment
	if multi {
		next = append(next, existing...)
		next = append(next, generated...)
	} else {
		next = append(next, generated...)
	}
	if err := trendlymodels.UpdateContentFields(brandID, contentID, map[string]any{
		"attachments": next,
	}); err != nil {
		log.Printf("ai image-gen attachments update: %v", err)
	}
}

func aspectToSize(ratio string) string {
	switch ratio {
	case "1:1":
		return "1024x1024"
	case "4:5":
		return "1024x1280"
	case "16:9":
		return "1792x1024"
	case "9:16":
		return "1024x1792"
	default:
		return "1024x1024"
	}
}

// ───────────────────────── content_gen dispatcher (called from ws.go) ─────────────────────────

func handleContentGenWS(req WSRequest) {
	switch req.Task {
	case "script":
		handleScriptGenWS(req)
	case "image":
		handleImageGenWS(req)
	default:
		wsErrorTo(req.ConnectionID, "unknown content task: "+req.Task)
	}
}

// ───────────────────────── S3 upload helpers ─────────────────────────

var s3Client *s3.S3

func getS3() *s3.S3 {
	if s3Client != nil {
		return s3Client
	}
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
	s3Client = s3.New(sess)
	return s3Client
}

func uploadBase64Image(b64, brandID string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", err
	}
	return uploadBytes(data, "image/png", brandID, "png")
}

// imageDownloadClient fetches hosted image results. It has an explicit timeout
// so a stalled download can't hang the synchronous WS chat turn until the lambda
// itself times out (which would drop the turn's `done` frame and leave the
// client stuck "streaming").
var imageDownloadClient = &http.Client{Timeout: 60 * time.Second}

func uploadFromURL(url, brandID string) (string, error) {
	httpResp, err := imageDownloadClient.Get(url)
	if err != nil {
		return "", err
	}
	defer httpResp.Body.Close()
	data, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return "", err
	}
	ct := httpResp.Header.Get("Content-Type")
	ext := "png"
	if strings.Contains(ct, "jpeg") {
		ext = "jpg"
	} else if strings.Contains(ct, "webp") {
		ext = "webp"
	}
	return uploadBytes(data, ct, brandID, ext)
}

// uploadBytes stores an image in S3 following the same convention as the
// /s3/v1/images pre-sign endpoint used by the apps (useAWSContext): same bucket,
// the shared `uploads/` prefix, and the CloudFront URL the apps already render.
// AI-generated images thus live alongside user uploads and need no special
// handling on the client.
func uploadBytes(data []byte, contentType, brandID, ext string) (string, error) {
	bucket := os.Getenv("IMAGE_S3_BUCKET_NAME")
	cdn := os.Getenv("IMAGE_CF_DISTRIBUTION_URL")
	if bucket == "" {
		return "", fmt.Errorf("IMAGE_S3_BUCKET_NAME not set")
	}
	filename := fmt.Sprintf("file_%d_ai_%s.%s", time.Now().Unix(), uuid.NewString(), ext)
	key := fmt.Sprintf("uploads/%s", filename)
	_, err := getS3().PutObject(&s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", err
	}
	if cdn != "" {
		return fmt.Sprintf("%s/uploads/%s", strings.TrimRight(cdn, "/"), filename), nil
	}
	return fmt.Sprintf("https://%s.s3.amazonaws.com/%s", bucket, key), nil
}

func decodePayload(in map[string]any, out any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}
