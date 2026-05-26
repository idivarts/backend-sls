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
	model := openrouter.ResolveModel(openrouter.TaskCaption, req.Model)

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
	model := openrouter.ResolveModel(openrouter.TaskHashtag, req.Model)

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
	model := openrouter.ResolveModel(openrouter.TaskScript, req.Model)

	user := fmt.Sprintf(
		"Write a video script for a %s. Topic: %s. Key message: %s. Tone: %s.\n\nReturn structured markdown with these sections:\n## Hook (first 3 seconds)\n## Body (3-4 scenes with cues)\n## CTA",
		p.VideoType, p.Topic, p.KeyMessage, p.Tone,
	)

	streamErr := openrouter.ChatCompletionStream(context.Background(), openrouter.ChatRequest{
		Model:    model,
		Messages: []openrouter.Message{{Role: "system", Content: sys}, {Role: "user", Content: user}},
	}, openrouter.StreamCallbacks{
		OnDelta: func(delta string) {
			wsSend(req.ConnectionID, map[string]any{"type": "token", "delta": delta})
		},
		OnDone: func(u *openrouter.Usage) {
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
	Description string `json:"description"`
	Style       string `json:"style"`
	AspectRatio string `json:"aspectRatio"`
	Count       int    `json:"count"`
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
	model := openrouter.ResolveModel(openrouter.TaskImage, req.Model)

	prompt := p.Description
	if p.Style != "" {
		prompt = fmt.Sprintf("%s. Style: %s.", p.Description, p.Style)
	}

	for i := 0; i < p.Count; i++ {
		wsSend(req.ConnectionID, map[string]any{
			"type":  "image_status",
			"index": i,
			"state": "generating",
		})
		resp, err := openrouter.GenerateImage(context.Background(), openrouter.ImageRequest{
			Model:  model,
			Prompt: prompt,
			Size:   size,
			N:      1,
		})
		if err != nil {
			wsErrorTo(req.ConnectionID, fmt.Sprintf("image %d failed: %v", i, err))
			continue
		}
		url := ""
		if len(resp.Data) > 0 {
			url = resp.Data[0].URL
			if url == "" && resp.Data[0].B64JSON != "" {
				if s3url, err := uploadBase64Image(resp.Data[0].B64JSON, p.BrandID); err == nil {
					url = s3url
				}
			} else if url != "" {
				if s3url, err := uploadFromURL(url, p.BrandID); err == nil {
					url = s3url
				}
			}
		}
		wsSend(req.ConnectionID, map[string]any{
			"type":  "image",
			"index": i,
			"s3Url": url,
		})
	}

	wsSend(req.ConnectionID, map[string]any{"type": "done"})
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

func uploadBytes(data []byte, contentType, brandID, ext string) (string, error) {
	bucket := os.Getenv("IMAGE_S3_BUCKET_NAME")
	cdn := os.Getenv("IMAGE_CF_DISTRIBUTION_URL")
	if bucket == "" {
		return "", fmt.Errorf("IMAGE_S3_BUCKET_NAME not set")
	}
	key := fmt.Sprintf("ai-gen/%s/%d_%s.%s", brandID, time.Now().Unix(), uuid.NewString(), ext)
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
		return fmt.Sprintf("%s/%s", strings.TrimRight(cdn, "/"), key), nil
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
