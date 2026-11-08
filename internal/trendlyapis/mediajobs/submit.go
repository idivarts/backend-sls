package mediajobs

// The one definition of "start a generated clip".
//
// ── Why this is extracted ───────────────────────────────────────────────────
// Two surfaces now ask for a generated clip: the composition timeline and the
// Design Studio's scenes. The validation in front of it is the part that must
// not be duplicated — the registry's capability checks, the duration set, the
// wallet gate, the plan gate, the charge-on-acceptance — because a second copy
// is a second place for one of them to be forgotten, and the one that gets
// forgotten is always a gate.
//
// So the HTTP handlers do nothing but translate: a request in, a refusal or a
// job id out. Where the clip LANDS is a target, and the only thing that differs
// between the two surfaces.
//
// ⚠️ A refusal from here has already decided its status and its `reason`. The
// caller must not re-interpret it: `tokens_exhausted` wants a top-up and
// `plan_locked` wants a tier, and collapsing them into "upgrade" sends half the
// users to the wrong screen.

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/mediagen"
)

// Target says where a finished clip belongs. Exactly one shape is filled in.
type Target struct {
	// A clip on a composition timeline.
	CompositionID string
	ClipID        string

	// A scene in a studio. The scene's stack is replaced with the clip when
	// the job resolves.
	StudioID string
	SceneID  string
}

func (t Target) isStudio() bool { return t.StudioID != "" && t.SceneID != "" }
func (t Target) isClip() bool   { return t.CompositionID != "" && t.ClipID != "" }

// VideoRequest is everything needed to start one generation.
type VideoRequest struct {
	BrandID   string
	ContentID string
	Target    Target

	ModelID string
	Prompt  string

	AspectRatio   string
	DurationSecs  int
	Resolution    string
	GenerateAudio bool

	StartFrameURL string
	EndFrameURL   string
	// ReferenceURLs carry likeness: a cast member's approved anchor still, so
	// every clip that person appears in is generated from the SAME face rather
	// than from each clip's own reading of a description.
	ReferenceURLs []string

	CreatedBy string
}

// Refusal is a decided answer: a status and the body to send.
type Refusal struct {
	Status int
	Body   map[string]any
}

func refuse(status int, body map[string]any) *Refusal {
	return &Refusal{Status: status, Body: body}
}

// Outcome is what a successful submit produced.
type Outcome struct {
	JobID  string
	Tokens int64
	Model  *trendlymodels.MediaModel
}

// SubmitVideo validates, gates, charges and starts one generated clip.
func SubmitVideo(ctx context.Context, req VideoRequest) (*Outcome, *Refusal) {
	if !req.Target.isStudio() && !req.Target.isClip() {
		return nil, refuse(http.StatusBadRequest, map[string]any{
			"error":   "no_target",
			"message": "A clip has to land somewhere.",
		})
	}

	// The registry decides what a model can do, not the caller. A request for
	// a capability the chosen model lacks is refused here rather than silently
	// dropped by the provider — the user pays either way, and a clip that
	// ignores the end frame they pinned is a clip they did not ask for.
	model, err := trendlymodels.GetMediaModel(ctx, req.ModelID)
	if err != nil || model == nil || !model.Enabled || model.Kind != "video" {
		return nil, refuse(http.StatusBadRequest, map[string]any{
			"error":   "unknown_model",
			"message": "That video model isn't available.",
		})
	}
	if r := checkCapabilities(model, req); r != nil {
		return nil, r
	}

	orgID := orgIDForBrand(req.BrandID)
	tokens := trendlymodels.GenerativeVideoTokenCost(model, req.DurationSecs)

	// Both gates refuse BEFORE the provider is called, because a generation is
	// charged the moment it is accepted.
	if locked, reason := videoLocked(orgID, model); locked {
		return nil, refuse(http.StatusPaymentRequired, map[string]any{
			"error":   "upgrade_required",
			"reason":  reason,
			"feature": "generative_video",
			"cost":    tokens,
		})
	}

	spec := mediagen.Spec{
		Kind:          mediagen.KindVideo,
		ModelID:       req.ModelID,
		Prompt:        req.Prompt,
		AspectRatio:   req.AspectRatio,
		DurationSecs:  req.DurationSecs,
		Resolution:    req.Resolution,
		NativeAudio:   model.NativeAudio,
		GenerateAudio: req.GenerateAudio,
	}
	if req.StartFrameURL != "" {
		spec.StartFrame = &mediagen.AssetRef{URL: req.StartFrameURL}
	}
	if req.EndFrameURL != "" {
		spec.EndFrame = &mediagen.AssetRef{URL: req.EndFrameURL}
	}
	for _, u := range req.ReferenceURLs {
		if u != "" {
			spec.ImageRefs = append(spec.ImageRefs, mediagen.AssetRef{URL: u})
		}
	}

	res, err := mediagen.Submit(ctx, model.Provider, spec)
	if err != nil {
		// Logged with the model and the provider's own words. The user gets a
		// sentence they can act on, which is deliberately not the provider's
		// error — but without this line a refused generation is a 502 with no
		// trace anywhere, and the only way to learn why is to spend another one.
		log.Printf("mediajobs: submit %s via %s failed: %v", req.ModelID, model.Provider, err)
		return nil, refuse(http.StatusBadGateway, map[string]any{
			"error":   "submit_failed",
			"message": "Couldn't start this clip. Please try again.",
		})
	}
	if res.JobID == "" {
		log.Printf("mediajobs: submit %s via %s returned no job id: %s",
			req.ModelID, model.Provider, res.Error)
		return nil, refuse(http.StatusBadGateway, map[string]any{
			"error":   "submit_failed",
			"message": res.Error,
		})
	}

	// Charged on acceptance, refunded if it fails. The provider bills on
	// acceptance too, and holding the charge until completion would let a
	// drained wallet queue unlimited work.
	if orgID != "" && tokens > 0 {
		if _, derr := trendlymodels.DeductTokens(orgID, tokens); derr != nil {
			log.Printf("mediajobs: deduct %d for org %s: %v", tokens, orgID, derr)
		}
	}

	job := &trendlymodels.MediaJob{
		ID:             res.JobID,
		Provider:       model.Provider,
		Kind:           "video",
		ModelID:        req.ModelID,
		Prompt:         req.Prompt,
		BrandID:        req.BrandID,
		ContentID:      req.ContentID,
		CompositionID:  req.Target.CompositionID,
		ClipID:         req.Target.ClipID,
		StudioID:       req.Target.StudioID,
		SceneID:        req.Target.SceneID,
		Status:         trendlymodels.MediaJobPending,
		ChargedTokens:  tokens,
		OrganizationID: orgID,
		CreatedBy:      req.CreatedBy,
	}
	if err := trendlymodels.CreateMediaJob(ctx, job); err != nil {
		// The generation is already running and already charged. Losing the
		// record would orphan it, so this is loud.
		log.Printf("mediajobs: could not record job %s: %v", res.JobID, err)
		return nil, refuse(http.StatusBadGateway, map[string]any{
			"error":   "submit_failed",
			"message": "The clip started but we couldn't track it. Please contact support.",
		})
	}

	return &Outcome{JobID: res.JobID, Tokens: tokens, Model: model}, nil
}

// checkCapabilities refuses what the model cannot do.
//
// Every one of these refusals is a generation NOT paid for. The provider would
// answer 422 for most of them — the same money and a worse message — and would
// silently ignore the rest, which is worse still: a clip that cost full price
// and quietly dropped the start frame looks like the feature not working.
func checkCapabilities(model *trendlymodels.MediaModel, req VideoRequest) *Refusal {
	if req.StartFrameURL != "" && !model.StartFrame {
		return refuse(http.StatusBadRequest, map[string]any{
			"error":   "unsupported_capability",
			"message": "This model can't start from an image. Pick another, or remove the start frame.",
		})
	}
	if req.EndFrameURL != "" && !model.EndFrame {
		return refuse(http.StatusBadRequest, map[string]any{
			"error":   "unsupported_capability",
			"message": "This model can't end on an image. Pick another, or remove the end frame.",
		})
	}
	if len(req.ReferenceURLs) > 0 && !model.ImageRefs {
		// The one that matters most for consistency: without image references
		// a cast member's anchor still cannot be honoured, so the face drifts
		// between clips and nothing says why.
		return refuse(http.StatusBadRequest, map[string]any{
			"error": "unsupported_capability",
			"message": "This model can't take a reference image, so it can't keep a face " +
				"consistent. Pick a model that can, or drop the cast member.",
		})
	}

	// ⚠️ A KEYFRAME AND A REFERENCE IMAGE CANNOT BE COMBINED. The provider
	// forbids it outright, and the consequence for this product is bigger than
	// a validation error: CHAINING (which pins the previous clip's last frame
	// as the start frame) and ANCHORING A FACE (which references an approved
	// still) are mutually exclusive on the same clip. A beat can continue the
	// previous instant, or it can be guaranteed the same face — not both.
	//
	// Which is another reason a chain is not a way to hold a face across
	// separate beats: for a person appearing in two different moments, the
	// anchor is the mechanism, and the clips cannot also be chained.
	if (req.StartFrameURL != "" || req.EndFrameURL != "") && len(req.ReferenceURLs) > 0 {
		return refuse(http.StatusBadRequest, map[string]any{
			"error": "incompatible_capabilities",
			"message": "A clip can either continue from a frame or reference a face, not both. " +
				"Chain it for one unbroken movement, or anchor it on the face for a separate shot.",
		})
	}

	if secs := req.DurationSecs; secs > 0 {
		// An exact set beats the bounds where a model has one. Veo takes 4, 6
		// or 8 and refuses 5.
		if len(model.DurationsS) > 0 && !containsInt(model.DurationsS, secs) {
			return refuse(http.StatusBadRequest, map[string]any{
				"error": "unsupported_capability",
				"message": fmt.Sprintf("This model makes clips of %s seconds. Pick one of those.",
					joinInts(model.DurationsS)),
			})
		}
		if model.MinDurationS > 0 && secs < model.MinDurationS {
			return refuse(http.StatusBadRequest, map[string]any{
				"error":   "unsupported_capability",
				"message": "That clip is shorter than this model can make.",
			})
		}
		// Refused rather than clamped. A clip silently cut to the model's cap
		// is one the user paid for and did not ask for, and the timeline it was
		// sized for now has a gap.
		if model.MaxDurationS > 0 && secs > model.MaxDurationS {
			return refuse(http.StatusBadRequest, map[string]any{
				"error":   "unsupported_capability",
				"message": "That clip is longer than this model can make. Split it across two clips.",
			})
		}
	}
	return nil
}
