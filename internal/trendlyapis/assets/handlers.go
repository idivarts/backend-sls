// Package assets serves the brand's media library.
//
// One endpoint, read by every surface that offers the user something to pick:
// the reference tray, the assets panel, the canvas drop target, the chat
// paperclip, the inspector's slots. They used to each have their own idea of
// what was available — the tray showed this content's attachments, the AI tool
// scanned a year of posts, and nothing showed a stock photo's credit at all.
package assets

import (
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/internal/trendlyapis/rendering"
)

// ListAssets returns the brand's library.
// GET /api/v2/brands/:brandId/assets?source=&limit=
func ListAssets(c *gin.Context) {
	brandID := c.Param("brandId")
	// Reading the library is reading the brand's own media; a viewer who can
	// see the content calendar can see what is in it.
	if _, ok := middlewares.RequireFeaturePrivilege(c, brandID,
		trendlymodels.FeatureContentCalendar, trendlymodels.PrivCalendarView); !ok {
		return
	}

	source := c.Query("source")
	switch source {
	case "", trendlymodels.AssetSourceBrand, trendlymodels.AssetSourceUpload,
		trendlymodels.AssetSourceStock, trendlymodels.AssetSourceGenerated:
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown source"})
		return
	}

	limit, _ := strconv.Atoi(c.Query("limit"))
	if limit <= 0 || limit > 200 {
		limit = 60
	}

	list, err := trendlymodels.ListBrandAssets(c.Request.Context(), brandID, source, limit)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read the asset library"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": len(list), "assets": list})
}

type recordAssetRequest struct {
	Kind   string `json:"kind" binding:"required"`
	URL    string `json:"url" binding:"required"`
	Source string `json:"source"`

	ThumbURL   string `json:"thumbUrl"`
	PosterURL  string `json:"posterUrl"`
	MimeType   string `json:"mimeType"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	DurationMs int    `json:"durationMs"`
	Alt        string `json:"alt"`
	Title      string `json:"title"`

	OriginContentID string `json:"originContentId"`
}

// RecordAsset files an uploaded asset in the library.
// POST /api/v2/brands/:brandId/assets
//
// Called by the app after an upload lands in S3. Uploads are the one source the
// backend cannot observe for itself — stock and generated assets are recorded
// where they are produced, but a file the user picks goes straight from their
// device to the bucket.
//
// Provenance fields are deliberately NOT accepted from the client. An upload
// has none by definition, and a client able to set `hotlink` or `noAiInput`
// could either strand its own file at a URL we do not control or mark someone
// else's licensed photo as freely usable.
func RecordAsset(c *gin.Context) {
	brandID := c.Param("brandId")
	if _, ok := middlewares.RequireFeaturePrivilege(c, brandID,
		trendlymodels.FeatureContentCalendar, trendlymodels.PrivCalendarEditor); !ok {
		return
	}

	var req recordAssetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	switch req.Kind {
	case trendlymodels.AssetKindImage, trendlymodels.AssetKindVideo, trendlymodels.AssetKindAudio:
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown kind"})
		return
	}
	// Only an upload may be recorded this way; the other sources are written by
	// the code that produces them, with the provenance that goes with them.
	source := req.Source
	if source == "" {
		source = trendlymodels.AssetSourceUpload
	}
	if source != trendlymodels.AssetSourceUpload && source != trendlymodels.AssetSourceBrand {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "unsupported source",
			"message": "Stock and generated assets are recorded where they are created.",
		})
		return
	}

	id, created, err := trendlymodels.RecordAsset(c.Request.Context(), brandID, &trendlymodels.Asset{
		Kind:            req.Kind,
		Source:          source,
		URL:             req.URL,
		ThumbURL:        req.ThumbURL,
		PosterURL:       req.PosterURL,
		MimeType:        req.MimeType,
		Width:           req.Width,
		Height:          req.Height,
		DurationMs:      req.DurationMs,
		Alt:             req.Alt,
		Title:           req.Title,
		OriginContentID: req.OriginContentID,
		CreatedBy:       c.GetString("userId"),
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not record the asset"})
		return
	}
	// A new video gets looked at. Only on creation: re-recording the same URL
	// must not queue a second ingest of a file we have already read.
	if created && req.Kind == trendlymodels.AssetKindVideo {
		if err := rendering.SubmitIngest(brandID, id, req.URL); err != nil {
			// Not fatal. The footage is usable without ingest — it can be
			// trimmed and placed by hand — so a failed dispatch costs the
			// smart features, not the upload.
			log.Printf("assets: could not start ingest for %s: %v", id, err)
		}
	}

	// 200 rather than 201 when it was already there: re-recording the same URL
	// is what a retried upload does, and it is not an error.
	c.JSON(http.StatusOK, gin.H{"id": id, "created": created})
}

// DeleteAsset removes an asset from the library.
// DELETE /api/v2/brands/:brandId/assets/:assetId
//
// The file itself is left alone. A design or a published post may already point
// at it, and removing it from the library must not break media that is live.
func DeleteAsset(c *gin.Context) {
	brandID := c.Param("brandId")
	if _, ok := middlewares.RequireFeaturePrivilege(c, brandID,
		trendlymodels.FeatureContentCalendar, trendlymodels.PrivCalendarEditor); !ok {
		return
	}
	if err := trendlymodels.DeleteAsset(c.Request.Context(), brandID, c.Param("assetId")); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not remove the asset"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
