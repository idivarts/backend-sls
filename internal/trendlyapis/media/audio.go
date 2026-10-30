package media

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/elevenlabs"
	"github.com/idivarts/backend-sls/pkg/mys3"
)

// ── What a generation costs ─────────────────────────────────────────────────
//
// These were invented numbers — "$0.02 + $0.002 a second" for music, "$0.00018
// a character" for speech — and the music one was WRONG in the direction that
// matters: measured against the live API, a second of music costs 15 credits
// ($0.0033), so the old formula under-charged every track over ~20 seconds and
// sold a 60-second bed at roughly 80% of what it cost us.
//
// They now come from pkg/elevenlabs, which prices in the credits ElevenLabs
// actually bills and converts once. Kept as wrappers here because the AI tools
// and the HTTP handlers both call them by these names.
func MusicCostUSD(lengthMs int) float64 { return elevenlabs.MusicCostUSD(lengthMs) }

func TTSCostUSD(text string) float64 { return elevenlabs.TTSCostUSD(text) }

// STTCostUSD prices a transcription. New: footage ingest transcribes every
// uploaded video and nothing was charging for it.
func STTCostUSD(durationMs int) float64 { return elevenlabs.STTCostUSD(durationMs) }

type musicReq struct {
	Prompt       string `json:"prompt" binding:"required"`
	LengthMs     int    `json:"lengthMs"`
	Instrumental bool   `json:"instrumental"`
	// Title is a short name for the clip. The caller knows what it asked for;
	// deriving one from the prompt here would just reproduce the truncation
	// the picker already suffered from.
	Title string `json:"title"`
}

// ElevenLabs Music accepts 3s-10min. A caller asking for the length of its
// video can legitimately be outside that on both ends, and the cost scales
// with duration, so this clamps rather than letting a bad request through.
const (
	minMusicMs     = 3000
	maxMusicMs     = 600000
	defaultMusicMs = 20000
)

func ClampMusicMs(ms int) int {
	if ms <= 0 {
		return defaultMusicMs
	}
	if ms < minMusicMs {
		return minMusicMs
	}
	if ms > maxMusicMs {
		return maxMusicMs
	}
	return ms
}

type voiceoverReq struct {
	Text     string `json:"text" binding:"required"`
	VoiceID  string `json:"voiceId" binding:"required"`
	Language string `json:"language"`
	Model    string `json:"model"` // "multilingual" | "flash"
}

// GenerateMusic proxies ElevenLabs Music, stores the bytes, records a
// generatedAudio doc, and meters the org wallet.
func GenerateMusic(c *gin.Context) {
	brandID := c.Param("brandId")
	var req musicReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	orgID, _ := trendlymodels.OrgIDForBrand(brandID)
	if trendlymodels.TokensExhausted(orgID) {
		c.JSON(http.StatusPaymentRequired, gin.H{"error": "upgrade_required", "reason": "tokens_exhausted", "task": "music"})
		return
	}

	lengthMs := ClampMusicMs(req.LengthMs)
	audio, err := elevenlabs.GenerateMusic(elevenlabs.MusicRequest{
		Prompt:            req.Prompt,
		LengthMs:          lengthMs,
		ForceInstrumental: req.Instrumental,
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "audio generation failed", "detail": err.Error()})
		return
	}

	url, err := UploadAudio(audio, brandID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not store audio"})
		return
	}

	id, _ := trendlymodels.CreateGeneratedAudio(&trendlymodels.GeneratedAudio{
		BrandID:    brandID,
		Kind:       "music",
		Provider:   "elevenlabs",
		Prompt:     req.Prompt,
		Title:      strings.TrimSpace(req.Title),
		URL:        url,
		DurationMs: int64(lengthMs),
		Status:     "ready",
	})

	trendlymodels.MeterUSD(orgID, MusicCostUSD(lengthMs))

	c.JSON(http.StatusOK, gin.H{"id": id, "url": url, "kind": "music", "durationMs": lengthMs, "title": strings.TrimSpace(req.Title)})
}

// GenerateVoiceover proxies ElevenLabs TTS, stores the bytes, records the doc
// and meters the wallet.
func GenerateVoiceover(c *gin.Context) {
	brandID := c.Param("brandId")
	var req voiceoverReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	orgID, _ := trendlymodels.OrgIDForBrand(brandID)
	if trendlymodels.TokensExhausted(orgID) {
		c.JSON(http.StatusPaymentRequired, gin.H{"error": "upgrade_required", "reason": "tokens_exhausted", "task": "voiceover"})
		return
	}

	model := elevenlabs.TTSMultiV2
	if req.Model == "flash" {
		model = elevenlabs.TTSFlashV2_5
	}
	audio, err := elevenlabs.GenerateVoiceover(elevenlabs.TTSRequest{
		VoiceID:  req.VoiceID,
		Text:     req.Text,
		ModelID:  model,
		Language: req.Language,
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "voiceover generation failed", "detail": err.Error()})
		return
	}

	url, err := UploadAudio(audio, brandID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not store audio"})
		return
	}

	// ~15 chars/sec speaking rate → estimated duration.
	durMs := int64(float64(len([]rune(req.Text))) / 15.0 * 1000)
	id, _ := trendlymodels.CreateGeneratedAudio(&trendlymodels.GeneratedAudio{
		BrandID:    brandID,
		Kind:       "voiceover",
		Provider:   "elevenlabs",
		Prompt:     req.Text,
		VoiceID:    req.VoiceID,
		Language:   req.Language,
		URL:        url,
		DurationMs: durMs,
		Status:     "ready",
	})

	trendlymodels.MeterUSD(orgID, TTSCostUSD(req.Text))

	c.JSON(http.StatusOK, gin.H{"id": id, "url": url, "kind": "voiceover", "durationMs": durMs})
}

// ListVoices proxies the ElevenLabs voice catalogue for the picker.
func ListVoices(c *gin.Context) {
	voices, err := elevenlabs.ListVoices()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not load voices", "detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"voices": voices})
}

// ListGeneratedAudio returns a brand's generated audio (optionally filtered by
// ?kind=music|voiceover) for the audio library.
func ListGeneratedAudio(c *gin.Context) {
	brandID := c.Param("brandId")
	items, err := trendlymodels.ListGeneratedAudio(brandID, c.Query("kind"), 50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"audio": items})
}

// UploadAudio stores generated audio bytes in the attachment bucket under the
// audio/ prefix and returns the CloudFront URL (same convention as image/video
// uploads so the app plays it directly). Exported so the AI audio tools reuse it.
func UploadAudio(a *elevenlabs.Audio, brandID string) (string, error) {
	bucket := os.Getenv("ATTACHMENT_S3_BUCKET_NAME")
	cdn := os.Getenv("ATTACHMENT_CF_DISTRIBUTION_URL")
	if bucket == "" {
		return "", fmt.Errorf("ATTACHMENT_S3_BUCKET_NAME not set")
	}
	filename := fmt.Sprintf("file_%d_audio_%s.%s", time.Now().Unix(), uuid.NewString(), a.Ext)
	key := fmt.Sprintf("audio/%s", filename)
	_, err := mys3.Client.PutObject(&s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(a.Data),
		ContentType: aws.String(a.ContentType),
	})
	if err != nil {
		return "", err
	}
	if cdn != "" {
		return fmt.Sprintf("%s/audio/%s", strings.TrimRight(cdn, "/"), filename), nil
	}
	return fmt.Sprintf("https://%s.s3.amazonaws.com/%s", bucket, key), nil
}
