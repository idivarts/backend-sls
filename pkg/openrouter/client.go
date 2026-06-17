package openrouter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const baseURL = "https://openrouter.ai/api/v1"

var (
	apiKey     = os.Getenv("OPENROUTER_API_KEY")
	httpClient = &http.Client{Timeout: 0}
	appReferer = "https://trendly.now"
	appTitle   = "Trendly"
)

type Message struct {
	Role string `json:"-"`
	// Content is the plain-text body. When ContentParts is non-empty it takes
	// precedence on the wire (the OpenAI-compatible multimodal array); Content is
	// still kept populated with the flattened text so text-only consumers and
	// history persistence keep working unchanged.
	Content string `json:"-"`
	// ContentParts carries multimodal content (text + image_url parts). When set,
	// MarshalJSON serializes `content` as the parts array; otherwise it falls back
	// to the plain Content string. This is the SINGLE shared multimodal path —
	// both chat vision input and image-to-image generation build ContentParts
	// (see UserMessageWithImages). Do not add a separate private parts path.
	ContentParts []ContentPart `json:"-"`
	Name         string        `json:"-"`
	ToolCalls    []ToolCall    `json:"-"`
	ToolCallID   string        `json:"-"`
	// Images is populated on assistant responses from image-generation models
	// (modalities: ["image","text"]). Each entry's image_url.url is either a
	// data URI ("data:image/png;base64,…") or an https URL.
	Images []OutputImage `json:"-"`
}

// ContentPart is one element of a multimodal message body. Type is "text" (Text
// set) or "image_url" (ImageURL set), matching the OpenAI/OpenRouter chat schema.
type ContentPart struct {
	Type     string        `json:"type"`
	Text     string        `json:"text,omitempty"`
	ImageURL *ImageURLData `json:"image_url,omitempty"`
}

// messageWire is the on-the-wire shape of a Message. `content` is `any` so it
// can be a plain string or a parts array depending on the message.
type messageWire struct {
	Role       string        `json:"role"`
	Content    any           `json:"content,omitempty"`
	Name       string        `json:"name,omitempty"`
	ToolCalls  []ToolCall    `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
	Images     []OutputImage `json:"images,omitempty"`
}

// MarshalJSON emits the multimodal parts array when ContentParts is set, else
// the plain content string. Keeps every other field intact.
func (m Message) MarshalJSON() ([]byte, error) {
	w := messageWire{
		Role:       m.Role,
		Name:       m.Name,
		ToolCalls:  m.ToolCalls,
		ToolCallID: m.ToolCallID,
		Images:     m.Images,
	}
	if len(m.ContentParts) > 0 {
		w.Content = m.ContentParts
	} else if m.Content != "" {
		w.Content = m.Content
	}
	return json.Marshal(w)
}

// UnmarshalJSON accepts `content` as either a string (the usual response shape)
// or a parts array. Text parts are flattened into Content so existing callers
// that read Message.Content keep working.
func (m *Message) UnmarshalJSON(data []byte) error {
	var raw struct {
		Role       string          `json:"role"`
		Content    json.RawMessage `json:"content"`
		Name       string          `json:"name"`
		ToolCalls  []ToolCall      `json:"tool_calls"`
		ToolCallID string          `json:"tool_call_id"`
		Images     []OutputImage   `json:"images"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	m.Role = raw.Role
	m.Name = raw.Name
	m.ToolCalls = raw.ToolCalls
	m.ToolCallID = raw.ToolCallID
	m.Images = raw.Images
	m.Content = ""
	m.ContentParts = nil
	if len(raw.Content) > 0 {
		var s string
		if err := json.Unmarshal(raw.Content, &s); err == nil {
			m.Content = s
		} else {
			var parts []ContentPart
			if err := json.Unmarshal(raw.Content, &parts); err == nil {
				m.ContentParts = parts
				for _, p := range parts {
					if p.Type == "text" {
						m.Content += p.Text
					}
				}
			}
		}
	}
	return nil
}

// UserMessageWithImages builds a user message whose body is a multimodal parts
// array (optional leading text, then one image_url part per URL). Content is
// also set to the text so text-only paths and history reads stay correct. Used
// by both chat vision input and image-to-image generation.
func UserMessageWithImages(text string, imageURLs []string) Message {
	parts := make([]ContentPart, 0, len(imageURLs)+1)
	if strings.TrimSpace(text) != "" {
		parts = append(parts, ContentPart{Type: "text", Text: text})
	}
	for _, u := range imageURLs {
		if u = strings.TrimSpace(u); u == "" {
			continue
		}
		parts = append(parts, ContentPart{Type: "image_url", ImageURL: &ImageURLData{URL: u}})
	}
	return Message{Role: "user", Content: text, ContentParts: parts}
}

type OutputImage struct {
	Type     string       `json:"type"`
	ImageURL ImageURLData `json:"image_url"`
}

type ImageURLData struct {
	URL string `json:"url"`
}

type ToolCall struct {
	// Index is only populated on streamed tool-call deltas. OpenRouter splits a
	// single tool call across many SSE chunks (id/name in the first, arguments
	// dribbled across the rest) and uses index to tell concurrent calls apart.
	Index    *int         `json:"index,omitempty"`
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function ToolCallFunc `json:"function"`
}

type ToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ChatRequest struct {
	Model          string          `json:"model"`
	Messages       []Message       `json:"messages"`
	Stream         bool            `json:"stream,omitempty"`
	Tools          []Tool          `json:"tools,omitempty"`
	Temperature    float64         `json:"temperature,omitempty"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
	Plugins        []Plugin        `json:"plugins,omitempty"`
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
	// Modalities requests non-text output. For image generation send
	// ["image","text"] — the generated image comes back on the assistant
	// message's Images field.
	Modalities []string `json:"modalities,omitempty"`
}

// ResponseFormat asks the model to constrain its output. Type is "json_object"
// (free-form JSON) or "json_schema" (validated against JSONSchema). Support
// varies by model — pair it with an explicit "return only JSON" instruction and
// parse defensively, since some providers (e.g. Anthropic) may ignore it.
type ResponseFormat struct {
	Type       string      `json:"type"`
	JSONSchema *JSONSchema `json:"json_schema,omitempty"`
}

type JSONSchema struct {
	Name   string         `json:"name"`
	Strict bool           `json:"strict,omitempty"`
	Schema map[string]any `json:"schema"`
}

type Plugin struct {
	ID string `json:"id"`
}

type ChatResponse struct {
	ID      string   `json:"id"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   *Usage   `json:"usage,omitempty"`
}

type Choice struct {
	Index        int      `json:"index"`
	Message      Message  `json:"message"`
	Delta        *Message `json:"delta,omitempty"`
	FinishReason string   `json:"finish_reason,omitempty"`
}

type Usage struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	Cost             float64 `json:"cost,omitempty"`
}

type StreamCallbacks struct {
	OnDelta    func(delta string)
	OnToolCall func(call ToolCall)
	OnDone     func(usage *Usage)
	OnError    func(err error)
}

func ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	req.Stream = false
	body, err := doRequest(ctx, "/chat/completions", req)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	var resp ChatResponse
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("decode openrouter response: %w", err)
	}
	return &resp, nil
}

func ChatCompletionStream(ctx context.Context, req ChatRequest, cb StreamCallbacks) error {
	req.Stream = true
	body, err := doRequest(ctx, "/chat/completions", req)
	if err != nil {
		return err
	}
	defer body.Close()

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var usage *Usage
	// Tool calls arrive fragmented across chunks; accumulate by index and emit
	// each as a single complete ToolCall once the stream ends.
	toolAccum := map[int]*ToolCall{}
	var toolOrder []int
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			break
		}

		var chunk ChatResponse
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
		for _, ch := range chunk.Choices {
			if ch.Delta == nil {
				continue
			}
			if ch.Delta.Content != "" && cb.OnDelta != nil {
				cb.OnDelta(ch.Delta.Content)
			}
			for _, tc := range ch.Delta.ToolCalls {
				idx := 0
				if tc.Index != nil {
					idx = *tc.Index
				}
				acc, ok := toolAccum[idx]
				if !ok {
					acc = &ToolCall{}
					toolAccum[idx] = acc
					toolOrder = append(toolOrder, idx)
				}
				if tc.ID != "" {
					acc.ID = tc.ID
				}
				if tc.Type != "" {
					acc.Type = tc.Type
				}
				if tc.Function.Name != "" {
					acc.Function.Name = tc.Function.Name
				}
				acc.Function.Arguments += tc.Function.Arguments
			}
		}
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		if cb.OnError != nil {
			cb.OnError(err)
		}
		return err
	}
	if cb.OnToolCall != nil {
		for _, idx := range toolOrder {
			cb.OnToolCall(*toolAccum[idx])
		}
	}
	if cb.OnDone != nil {
		cb.OnDone(usage)
	}
	return nil
}

type ImageRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Size   string `json:"size,omitempty"`
	N      int    `json:"n,omitempty"`
	// InputImages are URLs (https or data URI) of base/reference images. When
	// non-empty the request becomes image-to-image: the prompt + these images are
	// sent as a multimodal user message (via the shared Message.ContentParts
	// path). Empty → plain text-to-image, behaviour unchanged.
	InputImages []string `json:"-"`
}

type ImageResponse struct {
	Created int64       `json:"created"`
	Data    []ImageData `json:"data"`
}

type ImageData struct {
	URL     string `json:"url,omitempty"`
	B64JSON string `json:"b64_json,omitempty"`
}

// GenerateImage produces a single image. OpenRouter serves image generation
// through the chat-completions endpoint with modalities:["image","text"] (there
// is no stable OpenAI-style /images/generations route), so we issue a chat
// request and pull the image off the assistant message. The result is mapped
// back onto ImageResponse so callers stay agnostic to the transport: data-URI
// images populate B64JSON, hosted images populate URL.
func GenerateImage(ctx context.Context, req ImageRequest) (*ImageResponse, *Usage, error) {
	prompt := req.Prompt
	if req.Size != "" {
		prompt = fmt.Sprintf("%s\n\nGenerate the image at %s resolution (matching that aspect ratio).", prompt, req.Size)
	}

	// Image-to-image when base images are supplied, else plain text-to-image.
	userMsg := Message{Role: "user", Content: prompt}
	if len(req.InputImages) > 0 {
		userMsg = UserMessageWithImages(prompt, req.InputImages)
	}

	chatResp, err := ChatCompletion(ctx, ChatRequest{
		Model:      req.Model,
		Messages:   []Message{userMsg},
		Modalities: []string{"image", "text"},
	})
	if err != nil {
		return nil, nil, err
	}

	out := &ImageResponse{}
	for _, ch := range chatResp.Choices {
		for _, img := range ch.Message.Images {
			url := strings.TrimSpace(img.ImageURL.URL)
			if url == "" {
				continue
			}
			if strings.HasPrefix(url, "data:") {
				// data:<mime>;base64,<payload>
				if idx := strings.Index(url, ","); idx >= 0 {
					out.Data = append(out.Data, ImageData{B64JSON: url[idx+1:]})
					continue
				}
			}
			out.Data = append(out.Data, ImageData{URL: url})
		}
	}
	if len(out.Data) == 0 {
		return nil, nil, fmt.Errorf("model %q returned no image", req.Model)
	}
	return out, chatResp.Usage, nil
}

func doRequest(ctx context.Context, path string, payload interface{}) (io.ReadCloser, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("OPENROUTER_API_KEY is not set")
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	if ctx == nil {
		ctx = context.Background()
	}
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, baseURL+path, bytes.NewReader(buf))
	if err != nil {
		cancel()
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("HTTP-Referer", appReferer)
	httpReq.Header.Set("X-Title", appTitle)

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		cancel()
		return nil, err
	}
	if resp.StatusCode >= 300 {
		defer resp.Body.Close()
		errBody, _ := io.ReadAll(resp.Body)
		cancel()
		return nil, fmt.Errorf("openrouter status %d: %s", resp.StatusCode, string(errBody))
	}

	return &cancelCloser{ReadCloser: resp.Body, cancel: cancel}, nil
}

type cancelCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelCloser) Close() error {
	err := c.ReadCloser.Close()
	if c.cancel != nil {
		c.cancel()
	}
	return err
}
