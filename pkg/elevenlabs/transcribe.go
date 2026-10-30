package elevenlabs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"
)

// ── Speech to text (Scribe) ─────────────────────────────────────────────────
//
// Used to transcribe uploaded footage with per-word timings, which is what
// makes "cut the dead air" and "start on the line about pricing" answerable at
// all. A transcript without timings is prose; a transcript WITH them is an
// index into the video.
//
// The API takes a `source_url`, so our own S3 url goes over as a string and
// the bytes never pass through this process. A 200MB upload would otherwise be
// downloaded and re-uploaded for no reason.

// transcribeModel is the FALLBACK, not the setting. The model id comes from the
// media registry where one is enabled — a provider's catalog changes without
// us, and footage ingest should not need a deploy to follow it — and this is
// what it degrades to when the registry is empty or unreachable.
const transcribeModel = "scribe_v2"

// Word is one token and when it was said.
type Word struct {
	Text string `json:"text"`
	// Start and End are SECONDS, as the API reports them. Converted to integer
	// milliseconds at the model boundary — every other time in this product is
	// integer ms, and two units in play is how a trim ends up a thousand times
	// too long.
	Start *float64 `json:"start"`
	End   *float64 `json:"end"`
	// Type is word | spacing | audio_event.
	Type string `json:"type"`
}

// Transcript is a finished transcription.
type Transcript struct {
	LanguageCode string `json:"language_code"`
	Text         string `json:"text"`
	Words        []Word `json:"words"`
	// AudioDurationSecs is the provider's own measure of length, which is
	// worth keeping as a cross-check against ffprobe's.
	AudioDurationSecs float64 `json:"audio_duration_secs"`
}

// TranscribeURL transcribes media we host, by URL.
//
// multipart/form-data even though no file is attached: the endpoint takes form
// fields, and sending JSON gets a validation error that reads like the url is
// wrong rather than the encoding.
func TranscribeURL(sourceURL, modelID string) (*Transcript, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("ELEVENLABS_API_KEY is not set")
	}
	if sourceURL == "" {
		return nil, fmt.Errorf("elevenlabs: a source url is required")
	}

	model := strings.TrimSpace(modelID)
	if model == "" {
		model = transcribeModel
	}

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for k, v := range map[string]string{
		"model_id":   model,
		"source_url": sourceURL,
		// Word granularity is the entire reason this call exists. Character
		// timings would be finer and are not useful here; `none` would make
		// the result prose.
		"timestamps_granularity": "word",
		// Speaker labels are not asked for: a brand's own footage is usually
		// one person, and diarization costs accuracy on short takes.
		"diarize": "false",
	} {
		if err := form.WriteField(k, v); err != nil {
			return nil, err
		}
	}
	if err := form.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+"/speech-to-text", &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("xi-api-key", apiKey)
	req.Header.Set("Content-Type", form.FormDataContentType())

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusAccepted {
		// 202 means the work went asynchronous and the result is coming to a
		// webhook. We do not ask for that, so receiving it means the request
		// was shaped differently than intended — better to say so than to
		// return an empty transcript that looks like silence.
		return nil, fmt.Errorf("elevenlabs: transcription was accepted asynchronously, which this caller does not handle")
	}
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusPaymentRequired {
			return nil, fmt.Errorf(
				"elevenlabs: transcription refused (402) — the workspace plan does not include speech to text")
		}
		return nil, fmt.Errorf("elevenlabs: transcription returned %d", resp.StatusCode)
	}

	var out Transcript
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Ms converts a word's seconds to integer milliseconds, with nil meaning zero.
//
// Exported so the model layer converts once, at the boundary, rather than
// every caller rediscovering that the provider speaks seconds.
func (w Word) Ms() (start, end int) {
	if w.Start != nil {
		start = int(*w.Start * 1000)
	}
	if w.End != nil {
		end = int(*w.End * 1000)
	}
	return start, end
}
