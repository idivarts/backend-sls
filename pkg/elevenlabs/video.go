package elevenlabs

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
)

// ── Video generation (the Flows Image & Video API) ──────────────────────────
//
// Asynchronous, unlike everything else in this package: POST returns a
// generation id immediately and the result is collected later, by webhook or by
// polling. That is not an implementation detail to hide — a clip takes minutes,
// so the timeline has to be able to show one as queued and resolve it later.
//
// ⚠️ The finished video's URL is SIGNED AND EXPIRES about an hour after the
// response that carried it. A design is a document people re-render weeks
// later, so the bytes must be copied to our own storage on completion; a clip
// that resolves to an expired URL is a composition that cannot be re-rendered.
// Fetching the generation again mints a fresh URL, which is the recovery path
// when a copy fails rather than a reason to skip copying.

// Video model ids. Veo is the default because ByteDance's models are disabled
// by default for API requests and need explicit approval per workspace — a
// model nobody has approved would fail every call with a configuration error
// that looks like a bug.
const (
	VideoVeo31      = "veo-3.1-generate-001"
	VideoVeo31Fast  = "veo-3.1-fast-generate-001"
	VideoSeedance2  = "bytedance-seedance-v2"
	VideoSeedance25 = "bytedance-seedance-v2.5"
)

// Generation statuses. `pending` and `generating` are both "still running";
// there is no single in-progress value, and treating only one of them as
// running is how a poll loop decides a live job has vanished.
const (
	VideoPending    = "pending"
	VideoGenerating = "generating"
	VideoCompleted  = "completed"
	VideoFailed     = "failed"
)

// ImageRef is a picture handed to a model: a start frame, an end frame or a
// reference. Exactly one of the three forms is set.
type ImageRef struct {
	Type string `json:"type"`
	// type = "asset"
	AssetID string `json:"asset_id,omitempty"`
	// type = "generation" — a previous generation of ours, reused by id.
	GenerationID string `json:"generation_id,omitempty"`
	// type = "inline_base64" — the bytes themselves, up to 25MB decoded.
	ContentBase64 string `json:"content_base64,omitempty"`
	MimeType      string `json:"mime_type,omitempty"`
}

// InlineImage builds the inline form, which is what our own S3-hosted frames
// have to use: the API takes bytes or ids of its own, and an arbitrary URL is
// neither.
func InlineImage(data []byte, mimeType string) ImageRef {
	return ImageRef{
		Type:          "inline_base64",
		ContentBase64: base64.StdEncoding.EncodeToString(data),
		MimeType:      mimeType,
	}
}

// VideoRequest is one clip to generate.
//
// Only the fields this product uses are modelled. The API also takes reference
// videos and audio for lipsync work, which belongs to a later phase — leaving
// them out keeps this struct honest about what has actually been exercised.
type VideoRequest struct {
	ModelID string `json:"model_id"`
	Prompt  string `json:"prompt,omitempty"`

	AspectRatio  string `json:"aspect_ratio,omitempty"`
	DurationSecs int    `json:"duration_secs,omitempty"`
	Resolution   string `json:"resolution,omitempty"`
	// GenerateAudio defaults to TRUE at the API, so it is a pointer here:
	// leaving it nil means "the model's default", while false has to be sent
	// explicitly to turn sound off.
	GenerateAudio *bool `json:"generate_audio,omitempty"`
	Seed          *int  `json:"seed,omitempty"`

	// StartFrame and EndFrame pin the first and last frame.
	//
	// ⚠️ The API forbids combining either with reference images, videos or
	// audio. Submit rejects that rather than dropping one silently — a caller
	// who pinned a last frame and got a clip that ignores it has paid for a
	// misunderstanding.
	StartFrame *ImageRef  `json:"start_frame,omitempty"`
	EndFrame   *ImageRef  `json:"end_frame,omitempty"`
	Images     []ImageRef `json:"images,omitempty"`

	// Webhook asks ElevenLabs to call the workspace's configured endpoints when
	// the generation reaches a terminal status.
	Webhook *WebhookTarget `json:"webhook,omitempty"`
}

// WebhookTarget selects which configured flows webhooks are notified.
type WebhookTarget struct {
	Type string   `json:"type"`
	IDs  []string `json:"ids,omitempty"`
}

// WebhookAll notifies every flows webhook configured on the workspace, which is
// what we want: the endpoint is configured once in the ElevenLabs dashboard
// rather than named per request, so a stage's own URL is a setting rather than
// something every call has to carry correctly.
func WebhookAll() *WebhookTarget { return &WebhookTarget{Type: "all"} }

// VideoGeneration is the state of one generation.
type VideoGeneration struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	// ContentURL is set only when completed, and is SIGNED — it expires about
	// an hour out. See the package note.
	ContentURL      string `json:"content_url,omitempty"`
	ContentMimeType string `json:"content_mime_type,omitempty"`
	// FailureReason is one of timeout, model_error, moderated,
	// invalid_parameters, dependency_failed, charging_failed, internal_error.
	FailureReason string `json:"failure_reason,omitempty"`
	ErrorMessage  string `json:"error_message,omitempty"`
}

// Running reports whether this generation is still in flight. Both pending and
// generating mean "not finished"; there is no single in-progress value.
func (g VideoGeneration) Running() bool {
	return g.Status == VideoPending || g.Status == VideoGenerating
}

// CreateVideo submits a generation and returns its id.
func CreateVideo(req VideoRequest) (*VideoGeneration, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("ELEVENLABS_API_KEY is not set")
	}
	if req.ModelID == "" {
		return nil, fmt.Errorf("elevenlabs: a video needs a model id")
	}
	// Enforced here rather than left to a 422, so the message names the
	// conflict instead of pointing at a field path.
	if (req.StartFrame != nil || req.EndFrame != nil) && len(req.Images) > 0 {
		return nil, fmt.Errorf(
			"elevenlabs: a start or end frame cannot be combined with reference images")
	}

	buf, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequest(http.MethodPost, baseURL+"/flows/video", bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("xi-api-key", apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	return decodeGeneration(httpReq, "create")
}

// GetVideo polls one generation.
//
// Also the way to refresh an expired content URL: a completed generation
// returns a newly signed one each time it is fetched.
func GetVideo(generationID string) (*VideoGeneration, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("ELEVENLABS_API_KEY is not set")
	}
	if generationID == "" {
		return nil, fmt.Errorf("elevenlabs: a generation id is required")
	}
	httpReq, err := http.NewRequest(http.MethodGet, baseURL+"/flows/video/"+generationID, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("xi-api-key", apiKey)
	return decodeGeneration(httpReq, "get")
}

func decodeGeneration(httpReq *http.Request, op string) (*VideoGeneration, error) {
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// 401/403 here is nearly always the key lacking the Image & Video
		// permission rather than being wrong, and video needs a Pro workspace
		// — worth saying, because "unauthorized" sends people to the wrong
		// place entirely.
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, fmt.Errorf(
				"elevenlabs: video %s refused (%d) — the key needs the Image & Video permission on a Pro workspace",
				op, resp.StatusCode)
		}
		return nil, fmt.Errorf("elevenlabs: video %s returned %d", op, resp.StatusCode)
	}
	var out VideoGeneration
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ParseVideoWebhook decodes a flows callback body.
//
// The payload is documented as matching the terminal GET response, so it
// decodes into the same struct — which is what keeps a webhook and a poll from
// ever disagreeing about what "done" looks like. A body that is neither is
// rejected rather than treated as a silent non-event.
func ParseVideoWebhook(body []byte) (*VideoGeneration, error) {
	var out VideoGeneration
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if out.ID == "" {
		return nil, fmt.Errorf("elevenlabs: webhook body carries no generation id")
	}
	switch out.Status {
	case VideoPending, VideoGenerating, VideoCompleted, VideoFailed:
	default:
		return nil, fmt.Errorf("elevenlabs: webhook body has an unknown status %q", out.Status)
	}
	return &out, nil
}
