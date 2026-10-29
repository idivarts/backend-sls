// Package elevenlabs is a thin server-side client for the ElevenLabs Music and
// Text-to-Speech APIs. The API key is read from ELEVENLABS_API_KEY and never
// leaves the backend (standing rule: the RN client must never call ElevenLabs
// directly). Generation returns raw audio bytes which the caller stores in S3.
package elevenlabs

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

const baseURL = "https://api.elevenlabs.io/v1"

var (
	apiKey     = os.Getenv("ELEVENLABS_API_KEY")
	httpClient = &http.Client{Timeout: 120 * time.Second}
)

// Model ids.
const (
	MusicV2      = "music_v2"
	MusicV1      = "music_v1"
	TTSMultiV2   = "eleven_multilingual_v2"
	TTSFlashV2_5 = "eleven_flash_v2_5"
)

// Audio is a generated clip: the raw bytes + its MIME type + extension.
type Audio struct {
	Data        []byte
	ContentType string
	Ext         string
}

// MusicRequest generates a background-music bed from a text prompt.
type MusicRequest struct {
	Prompt            string
	LengthMs          int  // 3000..600000; 0 lets ElevenLabs choose
	ForceInstrumental bool // true for beds under a voiceover
	ModelID           string
}

// GenerateMusic calls POST /v1/music and returns MP3 bytes. Body carries the
// prompt; output_format is a query param.
func GenerateMusic(req MusicRequest) (*Audio, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("ELEVENLABS_API_KEY is not set")
	}
	model := req.ModelID
	if model == "" {
		model = MusicV2
	}
	body := map[string]interface{}{
		"prompt":             req.Prompt,
		"model_id":           model,
		"force_instrumental": req.ForceInstrumental,
	}
	if req.LengthMs > 0 {
		body["music_length_ms"] = req.LengthMs
	}
	return postAudio("/music?output_format=mp3_44100_128", body)
}

// TTSRequest generates a voiceover from text using a specific voice.
type TTSRequest struct {
	VoiceID         string
	Text            string
	ModelID         string
	Language        string // ISO 639-1; optional
	Stability       float64
	SimilarityBoost float64
}

// GenerateVoiceover calls POST /v1/text-to-speech/{voice_id} and returns MP3.
func GenerateVoiceover(req TTSRequest) (*Audio, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("ELEVENLABS_API_KEY is not set")
	}
	if req.VoiceID == "" {
		return nil, fmt.Errorf("elevenlabs: voiceId is required")
	}
	model := req.ModelID
	if model == "" {
		model = TTSMultiV2
	}
	voiceSettings := map[string]interface{}{
		"stability":         orDefault(req.Stability, 0.5),
		"similarity_boost":  orDefault(req.SimilarityBoost, 0.75),
		"use_speaker_boost": true,
	}
	body := map[string]interface{}{
		"text":           req.Text,
		"model_id":       model,
		"voice_settings": voiceSettings,
	}
	if req.Language != "" {
		body["language_code"] = req.Language
	}
	path := fmt.Sprintf("/text-to-speech/%s?output_format=mp3_44100_128", url.PathEscape(req.VoiceID))
	return postAudio(path, body)
}

// Voice is a stock/cloned voice as returned by GET /v1/voices.
type Voice struct {
	VoiceID    string            `json:"voice_id"`
	Name       string            `json:"name"`
	Category   string            `json:"category"`
	Labels     map[string]string `json:"labels"`
	PreviewURL string            `json:"preview_url"`
}

// ListVoices calls GET /v1/voices and returns the available voices.
func ListVoices() ([]Voice, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("ELEVENLABS_API_KEY is not set")
	}
	httpReq, err := http.NewRequest(http.MethodGet, baseURL+"/voices", nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("xi-api-key", apiKey)
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs list voices: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("elevenlabs voices returned %s: %s", resp.Status, string(b))
	}
	var out struct {
		Voices []Voice `json:"voices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("elevenlabs decode voices: %w", err)
	}
	return out.Voices, nil
}

// postAudio POSTs a JSON body and returns the binary audio response.
func postAudio(path string, body map[string]interface{}) (*Audio, error) {
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs marshal: %w", err)
	}
	httpReq, err := http.NewRequest(http.MethodPost, baseURL+path, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("xi-api-key", apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "audio/mpeg")
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("elevenlabs returned %s: %s", resp.Status, string(b))
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs read body: %w", err)
	}
	return &Audio{Data: data, ContentType: "audio/mpeg", Ext: "mp3"}, nil
}

func orDefault(v, def float64) float64 {
	if v == 0 {
		return def
	}
	return v
}

// ── Timestamped TTS ─────────────────────────────────────────────────────────
//
// `/with-timestamps` returns the same audio plus character-level timings, which
// is what makes a voiceover line up with a video instead of merely fitting it.
// With the time each character is spoken, the end of each scene's line is a
// measured fact, so the SCENES can be retimed to the voice rather than the
// script being trimmed until it happens to fit.
//
// The alternative — one TTS clip per scene — gives exact boundaries too, but at
// the cost of a prosody reset and a seam at every cut. One take read straight
// through sounds like a person; five takes stitched together do not.

// CharTiming is one character and when it is spoken, in milliseconds.
type CharTiming struct {
	Char    string
	StartMs int
	EndMs   int
}

type timestampedResponse struct {
	AudioBase64 string `json:"audio_base64"`
	Alignment   struct {
		Characters                []string  `json:"characters"`
		CharacterStartTimesSecond []float64 `json:"character_start_times_seconds"`
		CharacterEndTimesSecond   []float64 `json:"character_end_times_seconds"`
	} `json:"alignment"`
}

// GenerateVoiceoverTimed calls the with-timestamps variant. The timings are
// index-matched to the characters of the text as the model received it, so the
// caller can map its own character offsets onto the audio.
//
// A caller that does not need timings should keep using GenerateVoiceover:
// this endpoint returns base64 rather than a byte stream, so it costs an extra
// copy of the clip in memory.
func GenerateVoiceoverTimed(req TTSRequest) (*Audio, []CharTiming, error) {
	if apiKey == "" {
		return nil, nil, fmt.Errorf("ELEVENLABS_API_KEY is not set")
	}
	if req.VoiceID == "" {
		return nil, nil, fmt.Errorf("elevenlabs: voiceId is required")
	}
	model := req.ModelID
	if model == "" {
		model = TTSMultiV2
	}
	body := map[string]interface{}{
		"text":     req.Text,
		"model_id": model,
		"voice_settings": map[string]interface{}{
			"stability":         orDefault(req.Stability, 0.5),
			"similarity_boost":  orDefault(req.SimilarityBoost, 0.75),
			"use_speaker_boost": true,
		},
	}
	if req.Language != "" {
		body["language_code"] = req.Language
	}
	path := fmt.Sprintf("/text-to-speech/%s/with-timestamps?output_format=mp3_44100_128",
		url.PathEscape(req.VoiceID))

	buf, err := json.Marshal(body)
	if err != nil {
		return nil, nil, fmt.Errorf("elevenlabs marshal: %w", err)
	}
	httpReq, err := http.NewRequest(http.MethodPost, baseURL+path, bytes.NewReader(buf))
	if err != nil {
		return nil, nil, err
	}
	httpReq.Header.Set("xi-api-key", apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, nil, fmt.Errorf("elevenlabs request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("elevenlabs returned %s: %s", resp.Status, string(b))
	}
	var out timestampedResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, nil, fmt.Errorf("elevenlabs decode: %w", err)
	}
	data, err := base64.StdEncoding.DecodeString(out.AudioBase64)
	if err != nil {
		return nil, nil, fmt.Errorf("elevenlabs audio decode: %w", err)
	}

	a := out.Alignment
	n := len(a.Characters)
	// The three arrays are documented as index-matched. Trusting that blindly
	// would panic on a short one, and a provider changing shape should degrade
	// to "no timings" rather than take the request down.
	if len(a.CharacterStartTimesSecond) < n {
		n = len(a.CharacterStartTimesSecond)
	}
	if len(a.CharacterEndTimesSecond) < n {
		n = len(a.CharacterEndTimesSecond)
	}
	timings := make([]CharTiming, 0, n)
	for i := 0; i < n; i++ {
		timings = append(timings, CharTiming{
			Char:    a.Characters[i],
			StartMs: int(a.CharacterStartTimesSecond[i] * 1000),
			EndMs:   int(a.CharacterEndTimesSecond[i] * 1000),
		})
	}
	return &Audio{Data: data, ContentType: "audio/mpeg", Ext: "mp3"}, timings, nil
}
