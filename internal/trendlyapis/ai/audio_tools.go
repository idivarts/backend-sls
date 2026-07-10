package ai

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/internal/trendlyapis/media"
	"github.com/idivarts/backend-sls/pkg/elevenlabs"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// Audio tools let the chat AI generate a background-music bed or a voiceover via
// ElevenLabs and attach it to the current (video) content. These call an
// external paid API, so they gate + meter the org token wallet directly.

const (
	toolGenerateMusic     = "generate_music"
	toolGenerateVoiceover = "generate_voiceover"
)

func audioServerTools() []openrouter.Tool {
	return []openrouter.Tool{
		openrouter.NewFunctionTool(
			toolGenerateMusic,
			"Generate a commercially-licensed background-music bed for this video content "+
				"from a text prompt (e.g. 'upbeat lofi, 20s, no vocals'). Use force instrumental "+
				"for beds under a voiceover. The music is baked into the video at render time.",
			openrouter.ObjectSchema(map[string]any{
				"prompt":       openrouter.StringProp("Describe the music: mood, genre, tempo, instruments."),
				"lengthMs":     openrouter.NumberProp("Length in milliseconds (3000–600000). Match the video length."),
				"instrumental": openrouter.EnumProp("Force instrumental (recommended under a voiceover).", []string{"true", "false"}),
			}, []string{"prompt"}),
		),
		openrouter.NewFunctionTool(
			toolGenerateVoiceover,
			"Generate an AI voiceover for this video content from the given text (usually the "+
				"script or caption), using a chosen voice. The voiceover is muxed over the music "+
				"bed at render time.",
			openrouter.ObjectSchema(map[string]any{
				"text":     openrouter.StringProp("The words to speak (script/caption)."),
				"voiceId":  openrouter.StringProp("The ElevenLabs voice id (from the voice picker)."),
				"language": openrouter.StringProp("Optional ISO 639-1 language code."),
			}, []string{"text", "voiceId"}),
		),
	}
}

type musicToolArgs struct {
	Prompt       string `json:"prompt"`
	LengthMs     int    `json:"lengthMs"`
	Instrumental string `json:"instrumental"`
}

func runGenerateMusic(ctx context.Context, brandID, contentID, orgID, arguments string) (string, error) {
	var a musicToolArgs
	if err := json.Unmarshal([]byte(arguments), &a); err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "could not parse arguments"}), nil
	}
	if strings.TrimSpace(a.Prompt) == "" {
		return jsonResult(map[string]any{"ok": false, "error": "prompt is required"}), nil
	}
	if aiTokensExhausted(orgID) {
		return jsonResult(map[string]any{"ok": false, "error": "out of AI tokens; tell the user to upgrade or add a top-up"}), nil
	}
	audio, err := elevenlabs.GenerateMusic(elevenlabs.MusicRequest{
		Prompt:            a.Prompt,
		LengthMs:          a.LengthMs,
		ForceInstrumental: a.Instrumental == "true",
	})
	if err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "music generation failed: " + err.Error()}), nil
	}
	url, err := media.UploadAudio(audio, brandID)
	if err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "could not store audio"}), nil
	}
	id, _ := trendlymodels.CreateGeneratedAudio(&trendlymodels.GeneratedAudio{
		BrandID: brandID, Kind: "music", Provider: "elevenlabs", Prompt: a.Prompt,
		URL: url, DurationMs: int64(a.LengthMs), Status: "ready",
	})
	if contentID != "" {
		attachMusic(brandID, contentID, id, url)
	}
	meterAIUsage(orgID, &openrouter.Usage{Cost: media.MusicCostUSD(a.LengthMs)})
	return jsonResult(map[string]any{"ok": true, "audioId": id, "url": url,
		"note": "music generated and attached; it will be baked into the video on render."}), nil
}

type voiceoverToolArgs struct {
	Text     string `json:"text"`
	VoiceID  string `json:"voiceId"`
	Language string `json:"language"`
}

func runGenerateVoiceover(ctx context.Context, brandID, contentID, orgID, arguments string) (string, error) {
	var a voiceoverToolArgs
	if err := json.Unmarshal([]byte(arguments), &a); err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "could not parse arguments"}), nil
	}
	if strings.TrimSpace(a.Text) == "" || a.VoiceID == "" {
		return jsonResult(map[string]any{"ok": false, "error": "text and voiceId are required"}), nil
	}
	if aiTokensExhausted(orgID) {
		return jsonResult(map[string]any{"ok": false, "error": "out of AI tokens; tell the user to upgrade or add a top-up"}), nil
	}
	audio, err := elevenlabs.GenerateVoiceover(elevenlabs.TTSRequest{
		VoiceID: a.VoiceID, Text: a.Text, Language: a.Language,
	})
	if err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "voiceover generation failed: " + err.Error()}), nil
	}
	url, err := media.UploadAudio(audio, brandID)
	if err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "could not store audio"}), nil
	}
	id, _ := trendlymodels.CreateGeneratedAudio(&trendlymodels.GeneratedAudio{
		BrandID: brandID, Kind: "voiceover", Provider: "elevenlabs", Prompt: a.Text,
		VoiceID: a.VoiceID, Language: a.Language, URL: url, Status: "ready",
	})
	if contentID != "" {
		attachVoiceover(brandID, contentID, id, url)
	}
	meterAIUsage(orgID, &openrouter.Usage{Cost: media.TTSCostUSD(a.Text)})
	return jsonResult(map[string]any{"ok": true, "audioId": id, "url": url,
		"note": "voiceover generated and attached; it will be muxed over the music at render."}), nil
}

// attachMusic / attachVoiceover merge the generated track into content.Audio,
// preserving the other track.
func attachMusic(brandID, contentID, id, url string) {
	audio := loadContentAudio(brandID, contentID)
	audio.MusicID = id
	audio.MusicURL = url
	audio.DuckMusic = true
	_ = trendlymodels.UpdateContentFields(brandID, contentID, map[string]interface{}{"audio": audio})
}

func attachVoiceover(brandID, contentID, id, url string) {
	audio := loadContentAudio(brandID, contentID)
	audio.VoiceoverID = id
	audio.VoiceoverURL = url
	_ = trendlymodels.UpdateContentFields(brandID, contentID, map[string]interface{}{"audio": audio})
}

func loadContentAudio(brandID, contentID string) *trendlymodels.ContentAudio {
	if c, err := trendlymodels.GetContent(brandID, contentID); err == nil && c.Audio != nil {
		return c.Audio
	}
	return &trendlymodels.ContentAudio{}
}
