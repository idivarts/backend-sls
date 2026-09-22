// Package elevenlabs is a thin server-side client for the ElevenLabs Music and
// Text-to-Speech APIs. The API key is read from ELEVENLABS_API_KEY and never
// leaves the backend (standing rule: the RN client must never call ElevenLabs
// directly). Generation returns raw audio bytes which the caller stores in S3.
package elevenlabs

import (
	"bytes"
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
	Prompt           string
	LengthMs         int  // 3000..600000; 0 lets ElevenLabs choose
	ForceInstrumental bool // true for beds under a voiceover
	ModelID          string
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
		"prompt":            req.Prompt,
		"model_id":          model,
		"force_instrumental": req.ForceInstrumental,
	}
	if req.LengthMs > 0 {
		body["music_length_ms"] = req.LengthMs
	}
	return postAudio("/music?output_format=mp3_44100_128", body)
}

// TTSRequest generates a voiceover from text using a specific voice.
type TTSRequest struct {
	VoiceID  string
	Text     string
	ModelID  string
	Language string // ISO 639-1; optional
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
		"text":          req.Text,
		"model_id":      model,
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
