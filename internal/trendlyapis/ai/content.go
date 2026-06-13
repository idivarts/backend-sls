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

	sys := buildSystemPrompt(brand, "content", req.BrandID, req.ContextID, "")
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

	sys := buildSystemPrompt(brand, "content", req.BrandID, req.ContextID, "")
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
	sys := buildSystemPrompt(brand, "content", p.BrandID, p.ContextID, "")
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

type imagePayload struct {
	BrandID     string `json:"brandId"`
	ContextID   string `json:"contextId"` // content doc id — where results + status are persisted
	Description string `json:"description"`
	Style       string `json:"style"`
	AspectRatio string `json:"aspectRatio"`
	Count       int    `json:"count"`
	Multi       bool   `json:"multi"` // carousel: append; single: replace the image
}

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

	size := aspectToSize(p.AspectRatio)
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

	prompt := p.Description
	if p.Style != "" {
		prompt = fmt.Sprintf("%s. Style: %s.", p.Description, p.Style)
	}

	// Mark the job running on the content doc up-front so the brand app shows
	// progress immediately and can recover the result even if this socket drops.
	setImageGenStatus(p.BrandID, p.ContextID, map[string]any{
		"status":         "generating",
		"prompt":         p.Description,
		"error":          "",
		"requestedCount": p.Count,
		"completedCount": 0,
		"startedAt":      time.Now().UnixMilli(),
	})

	// Snapshot existing attachments once so generated images can be appended
	// (carousel) or replace the image (single) and persisted incrementally.
	var existing []trendlymodels.ContentAttachment
	if p.ContextID != "" {
		if ct, err := trendlymodels.GetContent(p.BrandID, p.ContextID); err == nil && ct != nil {
			existing = ct.Attachments
		}
	}
	generated := make([]trendlymodels.ContentAttachment, 0, p.Count)

	completed := 0
	var firstErr error
	var imgCost float64
	for i := 0; i < p.Count; i++ {
		wsSend(req.ConnectionID, map[string]any{
			"type":  "image_status",
			"index": i,
			"state": "generating",
		})
		resp, usage, err := openrouter.GenerateImage(context.Background(), openrouter.ImageRequest{
			Model:  model,
			Prompt: prompt,
			Size:   size,
			N:      1,
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

		url := ""
		if len(resp.Data) > 0 {
			if resp.Data[0].URL != "" {
				if s3url, uerr := uploadFromURL(resp.Data[0].URL, p.BrandID); uerr == nil {
					url = s3url
				} else if firstErr == nil {
					firstErr = uerr
				}
			} else if resp.Data[0].B64JSON != "" {
				if s3url, uerr := uploadBase64Image(resp.Data[0].B64JSON, p.BrandID); uerr == nil {
					url = s3url
				} else if firstErr == nil {
					firstErr = uerr
				}
			}
		}
		if url == "" {
			wsErrorTo(req.ConnectionID, fmt.Sprintf("image %d: upload failed", i))
			continue
		}

		generated = append(generated, trendlymodels.ContentAttachment{Type: "image", ImageURL: url})
		completed++

		// Persist incrementally: the image lands on the content doc regardless of
		// whether the originating socket is still connected.
		persistGeneratedImages(p.BrandID, p.ContextID, existing, generated, p.Multi)
		setImageGenStatus(p.BrandID, p.ContextID, map[string]any{
			"status":         "generating",
			"completedCount": completed,
		})

		wsSend(req.ConnectionID, map[string]any{
			"type":  "image",
			"index": i,
			"s3Url": url,
		})
	}

	meterAIUsage(orgID, &openrouter.Usage{Cost: imgCost})

	if completed == 0 {
		msg := "image generation failed"
		if firstErr != nil {
			msg = firstErr.Error()
		}
		setImageGenStatus(p.BrandID, p.ContextID, map[string]any{
			"status": "error",
			"error":  msg,
		})
	} else {
		setImageGenStatus(p.BrandID, p.ContextID, map[string]any{
			"status":         "done",
			"error":          "",
			"completedCount": completed,
		})
	}

	wsSend(req.ConnectionID, map[string]any{"type": "done"})
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

func uploadFromURL(url, brandID string) (string, error) {
	httpResp, err := http.Get(url)
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
