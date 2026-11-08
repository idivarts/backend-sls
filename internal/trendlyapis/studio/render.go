package studio

// Rendering a studio.
//
// ── What actually happens ───────────────────────────────────────────────────
// A studio is CONVERTED into a Composition and handed to the renderer that
// already exists. Not a second renderer: the composition path already gets the
// hard part right — the frame-grid arithmetic that a cross-fade depends on, so
// a clip's declared length and its encoded length cannot disagree — and two
// implementations of that is two places for a half-frame error to make a
// dissolve fade from nothing.
//
// ── Why the refusal happens here ────────────────────────────────────────────
// The worker would reject an unrenderable studio too, but by then the wallet
// has paid for an attempt that was never going to produce anything. So the same
// questions are asked before the charge, and the answer is a sentence the user
// can act on rather than a failed job.
//
// ⚠️ Two limitations are reported rather than hidden, and both are deliberate:
// a LAYERED scene cannot export yet (a composition clip is one full-frame
// source), and a stylised transition renders as a dissolve. The first blocks
// the render, because publishing only the base layer would ship something
// nobody approved. The second is a note, because the cuts still land in the
// right places.

import (
	"context"
	"log"
	"net/http"

	"cloud.google.com/go/firestore"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/internal/trendlyapis/rendering"
	"github.com/idivarts/backend-sls/pkg/mysentry"
)

type renderStudioRequest struct {
	// Force re-renders one that already has a url, and charges again — it is a
	// deliberate new request, not a retry.
	Force bool `json:"force"`
}

// RenderStudio queues a render of the whole studio.
// POST …/studios/:studioId/render
func RenderStudio(c *gin.Context) {
	brandID, contentID, studioID, ok := guard(c, true)
	if !ok {
		return
	}
	var req renderStudioRequest
	_ = c.ShouldBindJSON(&req)

	ctx := c.Request.Context()
	s, err := trendlymodels.GetStudio(ctx, brandID, contentID, studioID)
	if err != nil || s == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such studio"})
		return
	}

	// Already running: answer with the job rather than starting a second one.
	// Two renders of the same studio race to write the same url, and the loser
	// is charged for output nobody sees.
	if !req.Force && (s.RenderStatus == trendlymodels.DesignRenderQueued ||
		s.RenderStatus == trendlymodels.DesignRenderRendering) {
		c.JSON(http.StatusOK, gin.H{
			"status":  s.RenderStatus,
			"message": "This is already rendering.",
		})
		return
	}

	scenes, err := trendlymodels.ListStudioScenes(ctx, brandID, contentID, studioID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read the scenes"})
		return
	}
	media, err := resolveSceneMedia(ctx, brandID, scenes)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read the media"})
		return
	}

	renderable, problem, notes := trendlymodels.StudioRenderable(s, scenes, media)
	if !renderable {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "not_renderable",
			"message": problem,
		})
		return
	}

	tokens := trendlymodels.StudioRenderTokenCost(s, scenes, media)
	alreadyPaid := s.RenderChargedTokens > 0 && !req.Force

	// A user-initiated render is gated on the wallet: refusing here is honest,
	// and the app can say what it would have cost.
	if !alreadyPaid {
		orgID, _ := trendlymodels.OrgIDForBrand(brandID)
		if trendlymodels.TokensExhausted(orgID) {
			c.JSON(http.StatusPaymentRequired, gin.H{
				"error":   "upgrade_required",
				"reason":  "tokens_exhausted",
				"feature": "studio_render",
				"cost":    tokens,
			})
			return
		}
		if orgID != "" && tokens > 0 {
			if _, derr := trendlymodels.DeductTokens(orgID, tokens); derr == nil {
				_ = trendlymodels.UpdateStudio(ctx, brandID, contentID, studioID,
					[]firestore.Update{{Path: "renderChargedTokens", Value: tokens}})
			}
		}
	}

	// Queued BEFORE dispatch, so the app shows the job the instant this
	// returns rather than sitting on a stale state until a worker starts.
	if err := trendlymodels.MarkStudioRenderQueued(ctx, brandID, contentID, studioID, ""); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not queue the render"})
		return
	}

	if err := rendering.SubmitStudioRender(brandID, contentID, studioID); err != nil {
		// Logged and reported to Sentry, because the user-facing sentence is
		// deliberately vague and this was the ONLY record of why a render
		// refused. Every studio render on dev answered this 502 for want of
		// two environment variables, and the logs said nothing at all — the
		// cause had to be found by reading `submitBatchJob` and then checking
		// the deployed function's configuration by hand.
		log.Printf("studio render dispatch failed for %s/%s/%s: %v",
			brandID, contentID, studioID, err)
		mysentry.Capture(err, map[string]string{
			"op":       "studio.render.dispatch",
			"brandId":  brandID,
			"studioId": studioID,
		})
		_ = trendlymodels.MarkStudioRenderFailed(ctx, brandID, contentID, studioID,
			"We couldn't start the render. Please try again.")
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "dispatch_failed",
			"message": "We couldn't start the render. Please try again.",
		})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"status":      trendlymodels.DesignRenderQueued,
		"tokensSpent": gin.H{"charged": !alreadyPaid, "tokens": tokens},
		// Said with the acceptance, so a difference the user should know about
		// reaches them before they look at the output and wonder.
		"notes": notes,
	})
}

// RenderPreflight answers "can this render, and what would it cost" without
// spending anything.
// GET …/studios/:studioId/render-preflight
//
// The honest other half of the cost gate. A Render button that only finds out
// it cannot render by trying is a button that charges for the discovery.
func RenderPreflight(c *gin.Context) {
	brandID, contentID, studioID, ok := guard(c, false)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	s, err := trendlymodels.GetStudio(ctx, brandID, contentID, studioID)
	if err != nil || s == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such studio"})
		return
	}
	scenes, err := trendlymodels.ListStudioScenes(ctx, brandID, contentID, studioID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read the scenes"})
		return
	}
	media, err := resolveSceneMedia(ctx, brandID, scenes)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read the media"})
		return
	}

	renderable, problem, notes := trendlymodels.StudioRenderable(s, scenes, media)
	m := trendlymodels.MeasureStudio(s, scenes)

	c.JSON(http.StatusOK, gin.H{
		"renderable":  renderable,
		"problem":     problem,
		"notes":       notes,
		"tokens":      trendlymodels.StudioRenderTokenCost(s, scenes, media),
		"durationMs":  m.TotalMs,
		"sceneCount":  len(m.Scenes),
		"alreadyPaid": s.RenderChargedTokens > 0,
	})
}

// resolveSceneMedia reads every media row the scenes reference, once.
//
// One pass rather than per scene: the same still in four scenes is one row, and
// re-reading it four times is four round trips for one answer.
func resolveSceneMedia(
	ctx context.Context,
	brandID string,
	scenes []trendlymodels.StudioScene,
) (map[string]*trendlymodels.Asset, error) {
	ids := make([]string, 0, len(scenes))
	for _, sc := range scenes {
		ids = append(ids, sc.MediaIDs...)
		for _, l := range sc.Layers {
			if l.MediaID != "" {
				ids = append(ids, l.MediaID)
			}
		}
	}
	return trendlymodels.GetAssets(ctx, brandID, ids)
}
