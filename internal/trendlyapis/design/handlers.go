package design

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/internal/trendlyapis/rendering"
	"github.com/idivarts/backend-sls/pkg/mysentry"
)

// guard checks the caller may work on this brand's designs and returns the
// ids. Authoring a design is an edit of the brand's content, so the calendar
// EDITOR privilege is the bar for writes and VIEW for reads — the same bar
// the studio package uses, because it is the same act.
func guard(c *gin.Context, editor bool) (brandID, designID string, ok bool) {
	brandID = c.Param("brandId")
	priv := trendlymodels.PrivCalendarView
	if editor {
		priv = trendlymodels.PrivCalendarEditor
	}
	if _, allowed := middlewares.RequireFeaturePrivilege(c, brandID,
		trendlymodels.FeatureContentCalendar, priv); !allowed {
		return "", "", false
	}
	return brandID, c.Param("designId"), true
}

type createDesignRequest struct {
	Name string `json:"name"`
	// ContentID is OPTIONAL. Empty means a standalone design, which is the
	// normal case when Design Studio is opened from the drawer rather than
	// from a post. "Design with AI" on a content sets it.
	ContentID string `json:"contentId"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	FPS       int    `json:"fps"`
}

// CreateDesign makes an empty design.
// POST /api/design/brands/:brandId/designs
func CreateDesign(c *gin.Context) {
	brandID, _, ok := guard(c, true)
	if !ok {
		return
	}
	var req createDesignRequest
	_ = c.ShouldBindJSON(&req)

	// A sensible canvas rather than a required one. The overwhelmingly common
	// case is a vertical social video, and making the caller state it every
	// time buys nothing.
	if req.Width <= 0 || req.Height <= 0 {
		req.Width, req.Height = 1080, 1920
	}
	if req.FPS <= 0 {
		req.FPS = 30
	}
	if req.Name == "" {
		req.Name = "Untitled design"
	}

	d := &trendlymodels.Design{
		Name:      req.Name,
		ContentID: req.ContentID,
		ManagerID: c.GetString("userId"),
		Format: trendlymodels.DesignFormat{
			Width:  req.Width,
			Height: req.Height,
			FPS:    req.FPS,
		},
	}
	id, err := trendlymodels.CreateBrandDesign(c.Request.Context(), brandID, d)
	if err != nil {
		log.Printf("design create failed for %s: %v", brandID, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not create the design"})
		return
	}
	d.ID = id
	c.JSON(http.StatusOK, gin.H{"design": d})
}

// ListDesigns returns the brand's designs, newest first, optionally filtered
// to one content.
// GET /api/design/brands/:brandId/designs?contentId=…
func ListDesigns(c *gin.Context) {
	brandID, _, ok := guard(c, false)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	var (
		designs []trendlymodels.Design
		err     error
	)
	if contentID := c.Query("contentId"); contentID != "" {
		designs, err = trendlymodels.ListBrandDesignsForContent(ctx, brandID, contentID, 50)
	} else {
		designs, err = trendlymodels.ListBrandDesigns(ctx, brandID, 50)
	}
	if err != nil {
		log.Printf("design list failed for %s: %v", brandID, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read the designs"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"designs": designs})
}

// GetDesign returns one design.
// GET /api/design/brands/:brandId/designs/:designId
func GetDesign(c *gin.Context) {
	brandID, designID, ok := guard(c, false)
	if !ok {
		return
	}
	d, err := trendlymodels.GetBrandDesign(c.Request.Context(), brandID, designID)
	if err != nil {
		log.Printf("design get failed for %s/%s: %v", brandID, designID, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read the design"})
		return
	}
	if d == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such design"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"design": d})
}

type renderDesignRequest struct {
	// Force re-renders one that already has a url, and charges again — it is
	// a deliberate new request, not a retry.
	Force bool `json:"force"`
	// Quality is the real cost lever: draft while iterating, delivery for the
	// file that gets published. Defaults to delivery, because the common case
	// for pressing Render is wanting the finished thing.
	Quality string `json:"quality"`
}

// RenderDesign charges the wallet and queues the render.
// POST /api/design/brands/:brandId/designs/:designId/render
func RenderDesign(c *gin.Context) {
	brandID, designID, ok := guard(c, true)
	if !ok {
		return
	}
	var req renderDesignRequest
	_ = c.ShouldBindJSON(&req)
	if req.Quality != "draft" && req.Quality != "looks" && req.Quality != "delivery" {
		req.Quality = "delivery"
	}

	ctx := c.Request.Context()
	d, err := trendlymodels.GetBrandDesign(ctx, brandID, designID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read the design"})
		return
	}
	if d == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such design"})
		return
	}

	// Nothing to render yet. This is the state a design sits in between being
	// created and the director writing its project, and it is a user-visible
	// no-op rather than an error — so it says so, and does not charge.
	if d.ProjectPrefix == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "not_renderable",
			"message": "This design has nothing in it yet.",
		})
		return
	}

	// Already running: answer with the job rather than starting a second one.
	// Two renders of the same design race to write the same url, and the
	// loser is charged for output nobody sees.
	if !req.Force && (d.RenderStatus == trendlymodels.DesignRenderQueued ||
		d.RenderStatus == trendlymodels.DesignRenderRendering) {
		c.JSON(http.StatusOK, gin.H{
			"status":  d.RenderStatus,
			"message": "This is already rendering.",
		})
		return
	}

	// Already done and unchanged: say so rather than doing the work, and
	// charging for it, twice.
	if !req.Force && d.RenderStatus == trendlymodels.DesignRenderDone && d.RenderURL != "" {
		c.JSON(http.StatusOK, gin.H{"status": "done", "url": d.RenderURL})
		return
	}

	tokens := trendlymodels.DesignV3RenderTokenCost(d.DurationMs)

	// Gated on the wallet before any work is queued. Refusing here is honest,
	// and the app can say what it would have cost.
	orgID, _ := trendlymodels.OrgIDForBrand(brandID)
	if trendlymodels.TokensExhausted(orgID) {
		c.JSON(http.StatusPaymentRequired, gin.H{
			"error":   "upgrade_required",
			"reason":  "tokens_exhausted",
			"feature": "design_render",
			"cost":    tokens,
		})
		return
	}
	charged := false
	if orgID != "" && tokens > 0 {
		if _, derr := trendlymodels.DeductTokens(orgID, tokens); derr == nil {
			charged = true
		}
	}

	// From here on, every failure path must give the tokens back. The charge
	// happens before the work so the wallet cannot be raced, which means the
	// moment the work does not start we owe a refund — otherwise the user has
	// paid for a render that never existed. This is not hypothetical: it is
	// exactly what happened the first time a v3 render was fired on dev, when
	// dispatch refused and the wallet went to -2229 for nothing.
	refund := func() {
		if charged {
			_, _ = trendlymodels.RefundTokens(orgID, tokens)
		}
	}

	// Queued BEFORE dispatch, so the app shows the job the instant this
	// returns rather than sitting on a stale state until a worker starts.
	if err := trendlymodels.MarkBrandDesignRenderQueued(ctx, brandID, designID); err != nil {
		refund()
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not queue the render"})
		return
	}

	if err := rendering.SubmitDesignV3Render(brandID, designID, req.Quality, req.Force); err != nil {
		// Logged and reported, because the user-facing sentence is
		// deliberately vague and this would otherwise be the only record of
		// why a render refused. Every studio render on dev once answered a
		// 502 for want of two environment variables, and the logs said
		// nothing at all.
		log.Printf("design render dispatch failed for %s/%s: %v", brandID, designID, err)
		mysentry.Capture(err, map[string]string{
			"op":       "design.render.dispatch",
			"brandId":  brandID,
			"designId": designID,
		})
		refund()
		_ = trendlymodels.MarkBrandDesignRenderFailed(ctx, brandID, designID,
			"We couldn't start the render. Please try again.")
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not start the render"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "queued", "cost": tokens})
}

// PreviewDesign captures still frames across the film so the user can see it
// before paying to render it.
// POST /api/design/brands/:brandId/designs/:designId/preview
//
// ── Why this charges nothing ───────────────────────────────────────────────
// It is the answer to "a user can see and approve before a single token is
// spent on rendering". A preview that cost tokens would not be that answer:
// the user would be paying to find out whether it is worth paying, and the
// one thing they cannot do is judge a design they have not seen.
//
// It is not free to US — it runs the same Fargate task, seeking six frames
// instead of three hundred — so it is rate-limited by the in-flight check
// below rather than by a price. If the compute turns out to matter, the lever
// is the frame count, not a charge.
//
// ⚠️ Phase-1 token values are provisional (see organization.go PlanLimitsMap),
// and so is this zero. It is a deliberate product decision, not an oversight.
func PreviewDesign(c *gin.Context) {
	brandID, designID, ok := guard(c, true)
	if !ok {
		return
	}

	ctx := c.Request.Context()
	d, err := trendlymodels.GetBrandDesign(ctx, brandID, designID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read the design"})
		return
	}
	if d == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such design"})
		return
	}

	if d.ProjectPrefix == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "not_previewable",
			"message": "This design has nothing in it yet.",
		})
		return
	}

	// Already running: answer with the job rather than starting a second one.
	// Two previews of the same design would each overwrite the other's frames,
	// and the filmstrip the user ends up reading would be a mix of the two.
	if d.PreviewStatus == trendlymodels.DesignPreviewQueued ||
		d.PreviewStatus == trendlymodels.DesignPreviewRunning {
		c.JSON(http.StatusOK, gin.H{
			"status":  d.PreviewStatus,
			"message": "This is already being previewed.",
		})
		return
	}

	// Queued BEFORE dispatch, so the app shows the job the instant this
	// returns rather than sitting on a stale filmstrip until a worker starts.
	if err := trendlymodels.MarkBrandDesignPreviewQueued(ctx, brandID, designID); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not queue the preview"})
		return
	}

	if err := rendering.SubmitDesignV3Preview(brandID, designID); err != nil {
		log.Printf("design preview dispatch failed for %s/%s: %v", brandID, designID, err)
		mysentry.Capture(err, map[string]string{"op": "design.preview.dispatch", "brandId": brandID, "designId": designID})
		// No refund to make — nothing was charged — but the document must not
		// be left at "queued" for a job that will never run, or the spinner
		// never resolves and reads as slowness rather than breakage.
		_ = trendlymodels.MarkBrandDesignPreviewFailed(ctx, brandID, designID,
			"We couldn't start the preview. Please try again.")
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not start the preview"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"status": trendlymodels.DesignPreviewQueued})
}
