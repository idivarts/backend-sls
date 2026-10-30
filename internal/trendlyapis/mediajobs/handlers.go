package mediajobs

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/mediagen"
)

func containsInt(xs []int, want int) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

// joinInts renders a duration set the way the message reads it: "4, 6 or 8".
func joinInts(xs []int) string {
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		out = append(out, strconv.Itoa(x))
	}
	switch len(out) {
	case 0:
		return ""
	case 1:
		return out[0]
	}
	return strings.Join(out[:len(out)-1], ", ") + " or " + out[len(out)-1]
}

type generateClipRequest struct {
	CompositionID string `json:"compositionId" binding:"required"`
	ClipID        string `json:"clipId" binding:"required"`
	ModelID       string `json:"modelId" binding:"required"`
	Prompt        string `json:"prompt" binding:"required"`

	AspectRatio   string `json:"aspectRatio"`
	DurationSecs  int    `json:"durationSecs"`
	Resolution    string `json:"resolution"`
	GenerateAudio bool   `json:"generateAudio"`

	StartFrameURL string `json:"startFrameUrl"`
	EndFrameURL   string `json:"endFrameUrl"`
}

// GenerateClip submits a generated-video job for one clip on a timeline.
// POST /api/v2/brands/:brandId/contents/:contentId/clips/generate
//
// Returns 202 with the job id. The clip shows as queued on the strip until the
// job resolves — which is minutes away, so nothing here waits for it.
func GenerateClip(c *gin.Context) {
	brandID := c.Param("brandId")
	contentID := c.Param("contentId")

	if _, ok := middlewares.RequireFeaturePrivilege(c, brandID,
		trendlymodels.FeatureContentCalendar, trendlymodels.PrivCalendarEditor); !ok {
		return
	}

	var req generateClipRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// The registry decides what a model can do, not the caller. A request for a
	// capability the chosen model lacks is refused here rather than silently
	// dropped by the provider — the user pays either way, and a clip that
	// ignores the end frame they pinned is a clip they did not ask for.
	model, err := trendlymodels.GetMediaModel(c.Request.Context(), req.ModelID)
	if err != nil || model == nil || !model.Enabled || model.Kind != "video" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "unknown_model",
			"message": "That video model isn't available.",
		})
		return
	}
	if req.StartFrameURL != "" && !model.StartFrame {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "unsupported_capability",
			"message": "This model can't start from an image. Pick another, or remove the start frame.",
		})
		return
	}
	if req.EndFrameURL != "" && !model.EndFrame {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "unsupported_capability",
			"message": "This model can't end on an image. Pick another, or remove the end frame.",
		})
		return
	}
	if secs := req.DurationSecs; secs > 0 {
		// An exact set beats the bounds where a model has one. Veo takes 4, 6 or
		// 8 and refuses 5 — the provider's 422 is the same money and a worse
		// message, so the refusal happens here.
		if len(model.DurationsS) > 0 && !containsInt(model.DurationsS, secs) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "unsupported_capability",
				"message": fmt.Sprintf("This model makes clips of %s seconds. Pick one of those.",
					joinInts(model.DurationsS)),
			})
			return
		}
		if model.MinDurationS > 0 && secs < model.MinDurationS {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "unsupported_capability",
				"message": "That clip is shorter than this model can make.",
			})
			return
		}
		// Refused rather than clamped. A clip silently cut to the model's cap
		// is one the user paid for and did not ask for, and the timeline it
		// was sized for now has a gap.
		if model.MaxDurationS > 0 && secs > model.MaxDurationS {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "unsupported_capability",
				"message": "That clip is longer than this model can make. Split it across two clips.",
			})
			return
		}
	}

	orgID := orgIDForBrand(brandID)
	tokens := trendlymodels.GenerativeVideoTokenCost(model, req.DurationSecs)

	// Gated on the wallet, and on the plan the model asks for. Both refuse
	// BEFORE the provider is called, because a generation is charged the
	// moment it is accepted.
	if locked, reason := videoLocked(orgID, model); locked {
		c.JSON(http.StatusPaymentRequired, gin.H{
			"error":   "upgrade_required",
			"reason":  reason,
			"feature": "generative_video",
			"cost":    tokens,
		})
		return
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

	res, err := mediagen.Submit(c.Request.Context(), model.Provider, spec)
	if err != nil {
		// Logged with the model and the provider's own words. The user gets a
		// sentence they can act on, which is deliberately not the provider's
		// error — but without this line a refused generation is a 502 with no
		// trace anywhere, and the only way to learn why is to spend another
		// one.
		log.Printf("mediajobs: submit %s via %s failed: %v", req.ModelID, model.Provider, err)
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "submit_failed",
			"message": "Couldn't start this clip. Please try again.",
		})
		return
	}
	if res.JobID == "" {
		log.Printf("mediajobs: submit %s via %s returned no job id: %s",
			req.ModelID, model.Provider, res.Error)
		c.JSON(http.StatusBadGateway, gin.H{"error": "submit_failed", "message": res.Error})
		return
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
		BrandID:        brandID,
		ContentID:      contentID,
		CompositionID:  req.CompositionID,
		ClipID:         req.ClipID,
		Status:         trendlymodels.MediaJobPending,
		ChargedTokens:  tokens,
		OrganizationID: orgID,
		CreatedBy:      c.GetString("userId"),
	}
	if err := trendlymodels.CreateMediaJob(c.Request.Context(), job); err != nil {
		// The generation is already running and already charged. Losing the
		// record would orphan it, so this is loud.
		log.Printf("mediajobs: could not record job %s: %v", res.JobID, err)
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "submit_failed",
			"message": "The clip started but we couldn't track it. Please contact support.",
		})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"jobId":       res.JobID,
		"status":      trendlymodels.MediaJobPending,
		"tokensSpent": gin.H{"charged": tokens > 0, "tokens": tokens},
	})
}

// Webhook receives a provider's callback for a finished generation.
// POST /media/webhook/:provider
//
// The provider is taken from the ROUTE, never from the body: choosing a parser
// by sniffing the payload would let whoever sends the payload choose it, and
// these arrive from the open internet.
//
// Always answers 200. A provider that gets an error retries, and every failure
// mode here — an unknown job, a body we cannot parse — is one that retrying
// cannot fix. The polling sweep is what actually guarantees resolution.
func Webhook(c *gin.Context) {
	provider := c.Param("provider")
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false})
		return
	}

	res, err := mediagen.ParseWebhook(provider, body)
	if err != nil {
		log.Printf("mediajobs: webhook from %s: %v", provider, err)
		c.JSON(http.StatusOK, gin.H{"ok": false})
		return
	}

	job, err := trendlymodels.GetMediaJob(c.Request.Context(), res.JobID)
	if err != nil || job == nil {
		// Normal, not alarming: dev and prod share a provider workspace, so
		// each receives every callback the workspace sends — including the
		// other's.
		c.JSON(http.StatusOK, gin.H{"ok": true, "known": false})
		return
	}
	if rerr := Resolve(c.Request.Context(), job, res); rerr != nil {
		log.Printf("mediajobs: resolve %s from webhook: %v", job.ID, rerr)
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// videoLocked reports whether this org may generate video at all, and why not.
func videoLocked(orgID string, model *trendlymodels.MediaModel) (bool, string) {
	if orgID == "" {
		// A brand predating organizations is never metered and never blocked,
		// exactly as the AI token meter treats one.
		return false, ""
	}
	w, err := trendlymodels.GetTokenWallet(orgID)
	if err == nil && w != nil && (w.Balance+w.TopupBalance) <= 0 {
		return true, "tokens_exhausted"
	}
	if model.MinPlan != "" && !trendlymodels.OrgMeetsPlan(orgID, model.MinPlan) {
		return true, "plan_required"
	}
	return false, ""
}

func orgIDForBrand(brandID string) string {
	var b trendlymodels.Brand
	if err := b.Get(brandID); err != nil || b.OrganizationID == nil {
		return ""
	}
	return *b.OrganizationID
}
