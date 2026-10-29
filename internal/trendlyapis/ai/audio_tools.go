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
				"title":        openrouter.StringProp("A short name for this track, 2-4 words (e.g. \"Warm lofi bed\"). It is what the user picks from later, so make it distinguishable from their other tracks — not a restatement of the prompt."),
				"lengthMs":     openrouter.NumberProp("Length in milliseconds (3000–600000). Match the video length."),
				"instrumental": openrouter.EnumProp("Force instrumental (recommended under a voiceover).", []string{"true", "false"}),
			}, []string{"prompt"}),
		),
		audioFitTool(),
		alignScenesTool(),
		openrouter.NewFunctionTool(
			toolGenerateVoiceover,
			"Generate an AI voiceover for this video content from the given text (usually the "+
				"script or caption), using a chosen voice. The voiceover is muxed over the music "+
				"bed at render time.",
			openrouter.ObjectSchema(map[string]any{
				"text":     openrouter.StringProp("The words to speak (script/caption). Ignored when sceneTexts is given."),
				"voiceId":  openrouter.StringProp("The ElevenLabs voice id (from the voice picker)."),
				"language": openrouter.StringProp("Optional ISO 639-1 language code."),
				"sceneTexts": map[string]any{
					"type": "array",
					"description": "STRONGLY PREFERRED for a video: the spoken line for each scene, " +
						"in order, one entry per scene. Read as ONE take, so it still sounds like a " +
						"person — but the timings of each line are captured, which is what lets " +
						"align_scenes_to_voiceover retime the scenes to the voice afterwards. " +
						"Without this the voice and the picture drift apart and nothing can tell " +
						"you where.",
					"items": map[string]any{"type": "string"},
				},
			}, []string{"voiceId"}),
		),
	}
}

type musicToolArgs struct {
	Prompt       string `json:"prompt"`
	Title        string `json:"title"`
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
	lengthMs := media.ClampMusicMs(a.LengthMs)
	audio, err := elevenlabs.GenerateMusic(elevenlabs.MusicRequest{
		Prompt:            a.Prompt,
		LengthMs:          lengthMs,
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
		Title: strings.TrimSpace(a.Title),
		URL:   url, DurationMs: int64(lengthMs), Status: "ready",
	})
	if contentID != "" {
		attachMusic(brandID, contentID, id, url, strings.TrimSpace(a.Title))
	}
	meterAIUsage(orgID, &openrouter.Usage{Cost: media.MusicCostUSD(lengthMs)})
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
	if a.VoiceID == "" {
		return jsonResult(map[string]any{"ok": false, "error": "voiceId is required"}), nil
	}
	// Per-scene lines are joined into ONE take. Generating a clip per scene
	// would give exact boundaries too, but resets the prosody and leaves a
	// seam at every cut; one read straight through sounds like a person.
	sceneTexts := parseSceneTexts(arguments)
	text := strings.TrimSpace(a.Text)
	if len(sceneTexts) > 0 {
		text = strings.Join(sceneTexts, sceneJoiner)
	}
	if text == "" {
		return jsonResult(map[string]any{"ok": false, "error": "give either text or sceneTexts"}), nil
	}
	if aiTokensExhausted(orgID) {
		return jsonResult(map[string]any{"ok": false, "error": "out of AI tokens; tell the user to upgrade or add a top-up"}), nil
	}

	var (
		audio      *elevenlabs.Audio
		err        error
		boundaries []trendlymodels.SceneBoundary
		exact      bool
	)
	if len(sceneTexts) > 0 {
		var timings []elevenlabs.CharTiming
		audio, timings, err = elevenlabs.GenerateVoiceoverTimed(elevenlabs.TTSRequest{
			VoiceID: a.VoiceID, Text: text, Language: a.Language,
		})
		if err == nil {
			boundaries, exact = sceneBoundariesFrom(sceneTexts, sceneJoiner, timings)
		}
	} else {
		audio, err = elevenlabs.GenerateVoiceover(elevenlabs.TTSRequest{
			VoiceID: a.VoiceID, Text: text, Language: a.Language,
		})
	}
	if err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "voiceover generation failed: " + err.Error()}), nil
	}
	url, err := media.UploadAudio(audio, brandID)
	if err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "could not store audio"}), nil
	}
	id, _ := trendlymodels.CreateGeneratedAudio(&trendlymodels.GeneratedAudio{
		BrandID: brandID, Kind: "voiceover", Provider: "elevenlabs", Prompt: text,
		VoiceID: a.VoiceID, Language: a.Language, URL: url, Status: "ready",
	})
	if contentID != "" {
		attachVoiceover(brandID, contentID, id, url, boundaries, exact)
	}
	meterAIUsage(orgID, &openrouter.Usage{Cost: media.TTSCostUSD(text)})

	note := "voiceover generated and attached; it will be muxed over the music at render."
	if len(boundaries) > 0 {
		note = "Voiceover generated with per-scene timings. Call " + toolAlignScenes +
			" now so each scene lasts exactly as long as its own line."
	} else {
		note += " No per-scene timings were captured, so the voice and the picture may drift — " +
			"pass sceneTexts next time."
	}
	return jsonResult(map[string]any{"ok": true, "audioId": id, "url": url,
		"sceneTimings": len(boundaries), "exact": exact, "note": note}), nil
}

// attachMusic / attachVoiceover merge the generated track into content.Audio,
// preserving the other track.
func attachMusic(brandID, contentID, id, url, title string) {
	audio := loadContentAudio(brandID, contentID)
	audio.MusicID = id
	audio.MusicURL = url
	// The lane shows this. Without it the UI had nothing to print but a
	// truncated prompt, and every track read the same.
	if title != "" {
		audio.MusicTitle = title
	}
	audio.DuckMusic = true
	_ = trendlymodels.UpdateContentFields(brandID, contentID, map[string]interface{}{"audio": audio})
}

func attachVoiceover(brandID, contentID, id, url string, bounds []trendlymodels.SceneBoundary, exact bool) {
	audio := loadContentAudio(brandID, contentID)
	audio.VoiceoverID = id
	audio.VoiceoverURL = url
	// Stale boundaries are worse than none: they would retime the scenes to a
	// voiceover that no longer exists. Replaced wholesale, cleared when the
	// new take has none.
	audio.SceneBoundaries = bounds
	audio.SceneTimingsExact = exact
	if len(bounds) > 0 {
		audio.VoiceoverMs = int64(bounds[len(bounds)-1].EndMs)
	} else {
		audio.VoiceoverMs = 0
	}
	_ = trendlymodels.UpdateContentFields(brandID, contentID, map[string]interface{}{"audio": audio})
}

func loadContentAudio(brandID, contentID string) *trendlymodels.ContentAudio {
	if c, err := trendlymodels.GetContent(brandID, contentID); err == nil && c.Audio != nil {
		return c.Audio
	}
	return &trendlymodels.ContentAudio{}
}
