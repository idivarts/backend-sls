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
	"strings"
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
	// NativeAudio says whether the model can produce sound at ALL, which is
	// not the same question as whether this generation wants it.
	//
	// It has to be carried, because a model that cannot refuses the field
	// rather than ignoring it: Creatify Aurora answers `generate_audio` with
	// "Extra inputs are not permitted", so sending an explicit `false` — which
	// is what asking for silence looks like on the wire — fails every
	// generation it serves. Only a model that takes the field is told anything
	// about sound.
	NativeAudio bool

	// GenerateAudio asks a model that can produce sound to do so.
	//
	// A plain bool, not a pointer, and the zero value means NO AUDIO. The
	// providers default it to on, which is the wrong default here: a generated
	// clip that arrives with sound nobody asked for fights the composition's
	// own soundtrack in the mix, and the person hears two.
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
	// Alt is the source's own description, for the img alt text. A stock photo
	// arrives with one; generated media does not.
	Alt string
	// Attribution must travel with the asset and surface on publish where the
	// source requires it. Empty for generated media.
	Attribution string
	// SourceURL is the asset's page at its origin, for a credit link.
	SourceURL string
	// CreatorURL links the person who made it. Some sources require the credit
	// to point HERE rather than at the asset's page, and to carry a referral
	// tag — the driver is what knows which, so it arrives already formed.
	CreatorURL string

	// Hotlink means this asset must be used at the URL above and NOT copied to
	// our own storage. It inverts the rule stated at the top of this type, so a
	// caller that hosts unconditionally would be in breach rather than merely
	// wasteful.
	Hotlink bool
	// NoAIInput means this asset's licence forbids feeding it to a model. The
	// generation path refuses such a URL outright; see RestrictedRef.
	NoAIInput bool
	// TrackingRef is an opaque handle some sources require the app to report
	// back when the asset is actually USED, as opposed to merely found. Hand it
	// to ReportUsed; never interpret it here.
	TrackingRef string
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

// UsageReporter is implemented by a provider whose terms require the app to
// report that an asset was actually used — distinct from having searched for
// it. Optional: a provider that does not implement it is simply never asked.
type UsageReporter interface {
	ReportUsed(ctx context.Context, a Asset) error
}

// WebhookParser is implemented by a provider that can call us back when an
// asynchronous job finishes, instead of being polled.
//
// Optional, and a provider that implements it must still work under polling: a
// webhook that is misconfigured, blocked or simply never fires would otherwise
// strand every job forever, and the first anyone would know is a clip that sat
// as "queued" for a day.
type WebhookParser interface {
	ParseWebhook(body []byte) (Result, error)
}

// RefGuard is implemented by a provider whose assets may not be fed to a model.
// It reports whether a URL is one of its own restricted assets, and why, in a
// sentence a user can act on.
//
// A URL, rather than an Asset, because that is all a reference ever is by the
// time it reaches generation: the model hands back a string it was given. That
// is also why hotlinking turns out to matter twice — an asset kept at its
// origin carries its own provenance, while one copied into our storage would be
// indistinguishable from a user's upload and could not be guarded at all.
type RefGuard interface {
	RestrictedRef(url string) (bool, string)
}

var (
	// ErrNoProvider means nothing is registered that can serve the Spec.
	ErrNoProvider = errors.New("mediagen: no provider for this kind")
	// ErrRestrictedRef means a reference input is licensed in a way that
	// forbids model input. Its message is user-facing.
	ErrRestrictedRef = errors.New("mediagen: this image cannot be used as a generation reference")
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
	// Checked HERE rather than at each call site, because the seam is the one
	// place every generation already passes through. A licence restriction
	// enforced by convention is enforced until someone adds a caller.
	if why, bad := restrictedInput(s); bad {
		return Result{}, fmt.Errorf("%w: %s", ErrRestrictedRef, why)
	}
	return p.Submit(ctx, s)
}

// restrictedInput reports whether any reference input on the Spec comes from a
// source whose licence forbids model input, and the reason to show the user.
//
// Every slot that reaches a model is checked, not just ImageRefs: a start frame
// and a source video are model input in exactly the same sense, and the
// restriction is about the content, not the field it arrived in.
func restrictedInput(s Spec) (string, bool) {
	refs := make([]string, 0, len(s.ImageRefs)+3)
	for _, r := range s.ImageRefs {
		refs = append(refs, r.URL)
	}
	for _, r := range []*AssetRef{s.StartFrame, s.EndFrame, s.SourceVideo} {
		if r != nil {
			refs = append(refs, r.URL)
		}
	}
	if len(refs) == 0 {
		return "", false
	}

	for _, url := range refs {
		if bad, why := RestrictedRef(url); bad {
			return why, true
		}
	}
	return "", false
}

// RestrictedRef reports whether a URL may not be fed to a model, and why.
//
// Exported because not every generation goes through Submit yet — the image
// "enhance" path still calls its provider directly — and a licence gate that
// only covers some of the ways in is not a gate. Callers outside this package
// use it to refuse early, with the same sentence the seam would have produced.
func RestrictedRef(url string) (bool, string) {
	if strings.TrimSpace(url) == "" {
		return false, ""
	}
	mu.RLock()
	guards := make([]RefGuard, 0, len(providers))
	for _, p := range providers {
		if g, ok := p.(RefGuard); ok {
			guards = append(guards, g)
		}
	}
	mu.RUnlock()

	for _, g := range guards {
		if bad, why := g.RestrictedRef(url); bad {
			return true, why
		}
	}
	return false, ""
}

// RestrictedRefs is RestrictedRef over a list, returning the first reason.
func RestrictedRefs(urls []string) (bool, string) {
	for _, u := range urls {
		if bad, why := RestrictedRef(u); bad {
			return true, why
		}
	}
	return false, ""
}

// ParseWebhook decodes a provider's callback into a Result.
//
// The provider is named by the route rather than discovered from the body:
// deciding which parser to run by sniffing a payload means an attacker picks
// the parser, and these callbacks arrive from the open internet.
func ParseWebhook(provider string, body []byte) (Result, error) {
	p, ok := ByName(provider)
	if !ok {
		return Result{}, fmt.Errorf("%w: %q", ErrNoProvider, provider)
	}
	w, ok := p.(WebhookParser)
	if !ok {
		return Result{}, fmt.Errorf("%w: %q sends no callbacks", ErrUnsupported, provider)
	}
	return w.ParseWebhook(body)
}

// ReportUsed tells a provider that an asset was actually used, where its terms
// require that. A no-op for providers that do not implement UsageReporter, and
// for an asset carrying no TrackingRef.
//
// Deliberately separate from Submit: searching for a photo and PLACING one are
// different events, and reporting the first as the second would misstate which
// work is actually being used — which is the whole point of the endpoint.
func ReportUsed(ctx context.Context, provider string, a Asset) error {
	if a.TrackingRef == "" {
		return nil
	}
	p, ok := ByName(provider)
	if !ok {
		return fmt.Errorf("%w: %q", ErrNoProvider, provider)
	}
	r, ok := p.(UsageReporter)
	if !ok {
		return nil
	}
	return r.ReportUsed(ctx, a)
}

// Get polls a job on the named provider.
func Get(ctx context.Context, provider, jobID string) (Result, error) {
	p, ok := ByName(provider)
	if !ok {
		return Result{}, fmt.Errorf("%w: %q", ErrNoProvider, provider)
	}
	return p.Get(ctx, jobID)
}
