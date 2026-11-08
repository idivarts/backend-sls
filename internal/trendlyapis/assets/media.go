package assets

// The media collection's own endpoints.
//
// ListAssets (next door) answers "what has this brand ever attached", unioning
// in a derived view of old content attachments so nothing is invisible on day
// one. These answer the questions that only a real collection can:
//
//	GET    …/media            what can I put in a scene right now
//	GET    …/media/usage      how much of the bucket is this brand using
//	POST   …/media/:id/confirm   the bytes landed; measure them
//	POST   …/media/:id/archive   put it away, without deleting the bytes
//	POST   …/media/:id/frame     cut a still out of it
//
// The confirm endpoint is the one that looks odd and is not. A pre-signed
// upload goes from the user's device straight to S3, so the backend never sees
// it: the row is filed `pending` when the URL is handed out, and this is the
// call that promotes it. The size written is read back from S3 with a HEAD —
// never taken from the request — because it is meant to be billable, and a
// number the uploader reports is a number the uploader can understate.

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/internal/trendlyapis/rendering"
	"github.com/idivarts/backend-sls/pkg/mediastore"
)

// ListMedia returns the brand's pickable media.
// GET /api/v2/brands/:brandId/media?kind=&source=&q=&limit=
//
// Pickable means ready: a pending upload has no bytes yet and an archived row
// was deliberately put away, so neither is offered. That is the difference from
// ListAssets, which is a history and shows everything.
func ListMedia(c *gin.Context) {
	brandID := c.Param("brandId")
	if _, ok := middlewares.RequireFeaturePrivilege(c, brandID,
		trendlymodels.FeatureContentCalendar, trendlymodels.PrivCalendarView); !ok {
		return
	}

	kind := c.Query("kind")
	switch kind {
	case "", trendlymodels.AssetKindImage, trendlymodels.AssetKindVideo, trendlymodels.AssetKindAudio:
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown kind"})
		return
	}

	limit, _ := strconv.Atoi(c.Query("limit"))
	if limit <= 0 || limit > 200 {
		limit = 60
	}

	var (
		rows []trendlymodels.Asset
		err  error
	)
	if q := c.Query("q"); q != "" {
		rows, err = trendlymodels.SearchAssets(c.Request.Context(), brandID, q, kind, limit)
	} else {
		rows, err = trendlymodels.ListLibraryAssets(c.Request.Context(), brandID, kind, c.Query("source"), limit)
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read the media library"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": len(rows), "media": rows})
}

// MediaUsage returns the brand's storage rollup.
// GET /api/v2/brands/:brandId/media/usage
//
// Reporting only, as agreed — nothing is charged for storage yet. It is
// maintained and exposed from the first phase anyway, because a counter started
// later can only ever report "bytes since we started counting", and the first
// thing anyone asks of a storage figure is what it was last month.
func MediaUsage(c *gin.Context) {
	brandID := c.Param("brandId")
	if _, ok := middlewares.RequireFeaturePrivilege(c, brandID,
		trendlymodels.FeatureContentCalendar, trendlymodels.PrivCalendarView); !ok {
		return
	}
	usage, err := trendlymodels.GetBrandMediaUsage(c.Request.Context(), brandID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read storage usage"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"usage": usage,
		// Said out loud so no client renders it as a bill.
		"billable": false,
	})
}

type confirmMediaRequest struct {
	// Dimensions and duration are the client's, and they are the only numbers
	// it is trusted with: it has the decoded file in hand and S3 cannot tell
	// us how long a video is. The SIZE is not taken from here.
	Width      int `json:"width"`
	Height     int `json:"height"`
	DurationMs int `json:"durationMs"`
}

// ConfirmMedia promotes a pending upload once its bytes have landed.
// POST /api/v2/brands/:brandId/media/:mediaId/confirm
//
// Called by the app the moment its PUT returns. A row nobody confirms is swept
// later; a row confirmed against a missing object is marked failed here, which
// is the honest outcome for an upload that was cancelled halfway.
func ConfirmMedia(c *gin.Context) {
	brandID := c.Param("brandId")
	if _, ok := middlewares.RequireFeaturePrivilege(c, brandID,
		trendlymodels.FeatureContentCalendar, trendlymodels.PrivCalendarEditor); !ok {
		return
	}
	var req confirmMediaRequest
	// A body is optional: an image upload knows its dimensions, a document
	// does not, and neither should fail to confirm over it.
	_ = c.ShouldBindJSON(&req)

	mediaID := c.Param("mediaId")
	if err := mediastore.Confirm(c.Request.Context(), brandID, mediaID, trendlymodels.AssetMeasurement{
		Width:      req.Width,
		Height:     req.Height,
		DurationMs: req.DurationMs,
	}); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	// A newly confirmed video gets looked at, exactly as a recorded one does.
	if a, err := trendlymodels.GetAsset(c.Request.Context(), brandID, mediaID); err == nil &&
		a != nil && a.Kind == trendlymodels.AssetKindVideo && a.Ingest == nil {
		_ = rendering.SubmitIngest(brandID, mediaID, a.URL)
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ArchiveMedia puts media away without touching the bytes.
// POST /api/v2/brands/:brandId/media/:mediaId/archive
//
// Refused while anything still references it. That refusal is the whole point:
// a published post pointing at a file is a post that breaks if the file goes,
// and no amount of "are you sure?" makes that safe. `?force=true` is for the
// case the user genuinely owns — their own unreferenced media — and still does
// not delete the object.
func ArchiveMedia(c *gin.Context) {
	brandID := c.Param("brandId")
	if _, ok := middlewares.RequireFeaturePrivilege(c, brandID,
		trendlymodels.FeatureContentCalendar, trendlymodels.PrivCalendarEditor); !ok {
		return
	}
	force := c.Query("force") == "true"
	err := trendlymodels.ArchiveAsset(c.Request.Context(), brandID, c.Param("mediaId"), force)
	if errors.Is(err, trendlymodels.ErrAssetReferenced) {
		c.JSON(http.StatusConflict, gin.H{
			"error":   "media_referenced",
			"message": "Something still uses this. Remove it from those scenes first.",
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not archive the media"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type extractFrameRequest struct {
	// AtMs is how far in to cut. Omit it, or send -1, for the LAST frame —
	// which is what chaining wants and what the caller cannot compute, since
	// only the worker has ffprobe.
	AtMs *int `json:"atMs"`
}

// ExtractFrame cuts a still out of a video and files it as its own media.
// POST /api/v2/brands/:brandId/media/:mediaId/frame
//
// The thing that makes chaining work without a download-and-reupload round
// trip: the last frame of beat 2 becomes the start frame of beat 3, as a real
// library row with `origin.sourceMediaId` and `origin.atMs` on it, so the chain
// is legible weeks later instead of being an anonymous PNG.
//
// Asynchronous. The worker has ffmpeg; this hands it the job and returns.
func ExtractFrame(c *gin.Context) {
	brandID := c.Param("brandId")
	if _, ok := middlewares.RequireFeaturePrivilege(c, brandID,
		trendlymodels.FeatureContentCalendar, trendlymodels.PrivCalendarEditor); !ok {
		return
	}
	var req extractFrameRequest
	_ = c.ShouldBindJSON(&req)

	mediaID := c.Param("mediaId")
	src, err := trendlymodels.GetAsset(c.Request.Context(), brandID, mediaID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read the media"})
		return
	}
	if src == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such media"})
		return
	}
	if src.Kind != trendlymodels.AssetKindVideo {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "not_a_video",
			"message": "Only a video has frames to cut.",
		})
		return
	}

	atMs := -1
	if req.AtMs != nil {
		atMs = *req.AtMs
	}
	if err := rendering.SubmitFrameExtract(brandID, mediaID, src.URL, atMs); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "could not start the extraction",
			"message": "We couldn't cut that frame. Please try again.",
		})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "extracting", "sourceMediaId": mediaID, "atMs": atMs})
}
