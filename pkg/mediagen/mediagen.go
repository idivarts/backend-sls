// Package mediagen is one seam for every kind of generated media.
//
// It does for images, video, music and stock what pkg/openrouter does for text
// models: a single Spec in, a single Result out, and the provider behind it is
// an implementation detail nothing outside this package may see.
//
// The seam is the hedge, not a nicety. Generative-media providers are the least
// stable dependency in this product — the catalog churns, models retire on
// weeks of notice, and whole APIs have been discontinued mid-year. Every place
// that reaches past this interface to a provider's own types is a place that
// has to be found and rewritten the next time that happens, usually under time
// pressure. So: NO driver type may leak into a handler.
//
// What this package does NOT do: decide whether the user is allowed to spend
// money. Plan gating, token metering and the pre-flight estimate live with the
// callers that can actually show an upgrade prompt.
package mediagen

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// Kind is what is being made.
type Kind string

const (
	KindImage Kind = "image"
	KindVideo Kind = "video"
	KindMusic Kind = "music"
	KindVoice Kind = "voice"
	// KindStock is a search, not a generation. It shares the seam because from
	// a caller's point of view "get me a photograph of X" is one question with
	// two possible answers, and which one is cheaper changes over time.
	KindStock Kind = "stock"
)

// Status is where a job is. Generation is asynchronous for video and
// synchronous for images, so a driver may return Succeeded from Submit.
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

// AssetRef points at media we already host. Drivers receive these as inputs
// (a start frame, a reference image) and must never be handed a URL we do not
// control — a provider fetching from a third-party CDN is a dependency we
// cannot see and cannot re-run later.
type AssetRef struct {
	URL    string `json:"url"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

// Spec is one request for media.
type Spec struct {
	Kind    Kind
	ModelID string
	Prompt  string

	// StartFrame / EndFrame pin the first and last frame of a generated clip.
	// A model that does not support them must reject the Spec rather than
	// ignore the fields: silently dropping a keyframe produces a clip the user
	// paid for and did not ask for.
	StartFrame *AssetRef
	EndFrame   *AssetRef

	// ImageRefs are style or subject references. SourceVideo is the input for
	// the video-edit models.
	ImageRefs   []AssetRef
	SourceVideo *AssetRef

	DurationSecs int
	Resolution   string
	AspectRatio  string
	// GenerateAudio asks a model that can produce sound to do so.
	GenerateAudio bool
	// Count is how many variations to produce. Drivers clamp to what they allow.
	Count int
}

// Result is what came back. A synchronous driver fills Assets immediately; an
// asynchronous one returns a JobID and Status pending, and the caller resolves
// it through Get or a webhook.
type Result struct {
	JobID  string
	Status Status
	Assets []Asset
	// Error is set when Status is Failed. It is a message for a human, not a
	// provider error object — the seam exists so provider shapes stop here.
	Error string
	// Usage is what the call cost, where the provider reports it. Nil means
	// "not reported", which is NOT the same as free — a caller that meters must
	// fall back to the registry's estimate rather than charging nothing.
	Usage *Usage
}

// Usage is the cost of one call, normalised across providers.
type Usage struct {
	CostUSD float64
}

// Asset is one piece of produced media, still at the PROVIDER's URL. Copying it
// to our own storage is the caller's job and is not optional: a signed provider
// URL typically expires within the hour, and a design that outlives it cannot
// be re-rendered.
type Asset struct {
	URL      string
	MimeType string
	Width    int
	Height   int
	// Attribution must travel with the asset and surface on publish where the
	// source requires it. Empty for generated media.
	Attribution string
	// SourceURL is the asset's page at its origin, for a credit link.
	SourceURL string
}

// Provider is one backend for one or more Kinds.
type Provider interface {
	// Name is the stable id used in the model registry's `provider` field.
	Name() string
	// Supports reports whether this provider handles a Kind at all.
	Supports(k Kind) bool
	// Submit starts the work. A synchronous provider returns a Result that is
	// already Succeeded.
	Submit(ctx context.Context, s Spec) (Result, error)
	// Get polls an asynchronous job. Synchronous providers return
	// ErrNotAsynchronous.
	Get(ctx context.Context, jobID string) (Result, error)
}

var (
	// ErrNoProvider means nothing is registered that can serve the Spec.
	ErrNoProvider = errors.New("mediagen: no provider for this kind")
	// ErrNotAsynchronous is returned by Get on a provider whose work completes
	// inside Submit.
	ErrNotAsynchronous = errors.New("mediagen: provider has no async jobs")
	// ErrUnsupported means the provider handles the Kind but not this Spec —
	// an end frame on a model without keyframes, a duration past its cap. It is
	// deliberately an error rather than a silent clamp.
	ErrUnsupported = errors.New("mediagen: provider cannot satisfy this spec")
)

var (
	mu        sync.RWMutex
	providers = map[string]Provider{}
)

// Register adds a provider. Called from each driver's init, so importing a
// driver is what enables it — and a driver that is not imported cannot spend
// money by accident.
func Register(p Provider) {
	mu.Lock()
	defer mu.Unlock()
	providers[p.Name()] = p
}

// ByName returns a registered provider.
func ByName(name string) (Provider, bool) {
	mu.RLock()
	defer mu.RUnlock()
	p, ok := providers[name]
	return p, ok
}

// Registered lists provider names, sorted, for diagnostics.
func Registered() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(providers))
	for n := range providers {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Submit routes a Spec to the named provider.
//
// `provider` comes from the model registry row, never from a caller's guess —
// which is what keeps "which provider serves this model" a piece of
// configuration rather than a branch in a handler.
func Submit(ctx context.Context, provider string, s Spec) (Result, error) {
	p, ok := ByName(provider)
	if !ok {
		return Result{}, fmt.Errorf("%w: %q", ErrNoProvider, provider)
	}
	if !p.Supports(s.Kind) {
		return Result{}, fmt.Errorf("%w: %q does not serve %q", ErrNoProvider, provider, s.Kind)
	}
	return p.Submit(ctx, s)
}

// Get polls a job on the named provider.
func Get(ctx context.Context, provider, jobID string) (Result, error) {
	p, ok := ByName(provider)
	if !ok {
		return Result{}, fmt.Errorf("%w: %q", ErrNoProvider, provider)
	}
	return p.Get(ctx, jobID)
}
