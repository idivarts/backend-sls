// Package videos hands out pre-signed uploads for video files.
//
// ⚠️ See the note in internal/s3/images: the hand-out is the only moment the
// backend sees a user's own upload, so the library row is filed here, pending,
// and confirmed by the app afterwards.
//
// This target is the RAW bucket, whose `raw_videos/` prefix triggers the
// transcoding pipeline. Generated video must not come through here — it is
// already encoded, and re-transcoding it costs time and quality for nothing.
package videos

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/mediastore"
)

// Check if the filename has a video extension
func isVideoFile(filename string) bool {
	videoExtensions := []string{".mp4", ".mkv", ".avi", ".mov", ".flv", ".wmv", ".webm", ".mpeg"}

	// Get the file extension and convert it to lowercase
	ext := strings.ToLower(filepath.Ext(filename))

	// Check if the extension is in the list of video extensions
	for _, videoExt := range videoExtensions {
		if ext == videoExt {
			return true
		}
	}
	return false
}

func S3UploadHandler(ctx *gin.Context) {
	filename := ctx.Query("filename")
	if filename == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"message": "Filename is required"})
		return
	}
	if !isVideoFile(filename) {
		ctx.JSON(http.StatusBadRequest, gin.H{"message": "Invalid file extension"})
		return
	}

	out, err := mediastore.PresignUpload(ctx.Request.Context(), mediastore.Spec{
		BrandID:   ctx.Query("brandId"),
		Target:    mediastore.TargetVideo,
		Filename:  filename,
		Kind:      trendlymodels.AssetKindVideo,
		Source:    trendlymodels.AssetSourceUpload,
		ContentID: ctx.Query("contentId"),
		CreatedBy: ctx.GetString("userId"),
		Origin:    &trendlymodels.AssetOrigin{Kind: trendlymodels.AssetOriginUpload},
	})
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	// appleUrl and playUrl are the same address today; both are returned
	// because the apps' upload callers read them by name.
	ctx.JSON(http.StatusOK, gin.H{
		"uploadUrl": out.UploadURL,
		"appleUrl":  out.AppleURL,
		"playUrl":   out.PlayURL,
		"mediaId":   out.MediaID,
	})
}
