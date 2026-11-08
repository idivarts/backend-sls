package mediajobs

import (
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
//
// The validation, the gates and the charge all live in SubmitVideo, shared with
// the Design Studio's own generation. This handler only translates HTTP.
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

	out, refusal := SubmitVideo(c.Request.Context(), VideoRequest{
		BrandID:   brandID,
		ContentID: contentID,
		Target: Target{
			CompositionID: req.CompositionID,
			ClipID:        req.ClipID,
		},
		ModelID:       req.ModelID,
		Prompt:        req.Prompt,
		AspectRatio:   req.AspectRatio,
		DurationSecs:  req.DurationSecs,
		Resolution:    req.Resolution,
		GenerateAudio: req.GenerateAudio,
		StartFrameURL: req.StartFrameURL,
		EndFrameURL:   req.EndFrameURL,
		CreatedBy:     c.GetString("userId"),
	})
	if refusal != nil {
		// Passed through as decided. Re-interpreting it here would collapse
		// `tokens_exhausted` and `plan_locked` into one answer, and they want
		// opposite things from the user.
		c.JSON(refusal.Status, refusal.Body)
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"jobId":       out.JobID,
		"status":      trendlymodels.MediaJobPending,
		"tokensSpent": gin.H{"charged": out.Tokens > 0, "tokens": out.Tokens},
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
