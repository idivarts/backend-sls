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
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	Name       string     `json:"name,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
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
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Stream      bool      `json:"stream,omitempty"`
	Tools       []Tool    `json:"tools,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Plugins     []Plugin  `json:"plugins,omitempty"`
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
}

type ImageResponse struct {
	Created int64       `json:"created"`
	Data    []ImageData `json:"data"`
}

type ImageData struct {
	URL     string `json:"url,omitempty"`
	B64JSON string `json:"b64_json,omitempty"`
}

func GenerateImage(ctx context.Context, req ImageRequest) (*ImageResponse, error) {
	if req.N == 0 {
		req.N = 1
	}
	body, err := doRequest(ctx, "/images/generations", req)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	var resp ImageResponse
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("decode image response: %w", err)
	}
	return &resp, nil
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
