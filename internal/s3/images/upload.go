// Package images hands out pre-signed uploads for image files.
//
// ⚠️ The pre-sign goes through pkg/mediastore rather than straight to S3, and
// not for tidiness: the hand-out is the ONLY moment the backend is involved in
// a user's own upload. The bytes go from their device to the bucket without
// passing through here, so an object that is not filed at hand-out time is an
// object nothing will ever know about. When a brand is supplied, mediastore
// files a `pending` row now and the app confirms it once the PUT returns.
package images

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/mediastore"
)

// Check if the filename has an image extension
func isImageFile(filename string) bool {
	imageExtensions := []string{".jpg", ".jpeg", ".png", ".gif", ".bmp", ".tiff", ".svg", ".webp", ".heic"}

	// Get the file extension and convert it to lowercase
	ext := strings.ToLower(filepath.Ext(filename))

	// Check if the extension is in the list of image extensions
	for _, imgExt := range imageExtensions {
		if ext == imgExt {
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
	if !isImageFile(filename) {
		ctx.JSON(http.StatusBadRequest, gin.H{"message": "Invalid file extension"})
		return
	}

	// brandId is optional and backward compatible: the influencer app has no
	// brand, and an older brand-app build that does not send one still gets a
	// working upload — it just does not appear in the library until it does.
	out, err := mediastore.PresignUpload(ctx.Request.Context(), mediastore.Spec{
		BrandID:   ctx.Query("brandId"),
		Target:    mediastore.TargetImage,
		Filename:  filename,
		Kind:      trendlymodels.AssetKindImage,
		Source:    trendlymodels.AssetSourceUpload,
		ContentID: ctx.Query("contentId"),
		CreatedBy: ctx.GetString("userId"),
		Origin:    &trendlymodels.AssetOrigin{Kind: trendlymodels.AssetOriginUpload},
	})
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"uploadUrl": out.UploadURL,
		"imageUrl":  out.URL,
		"mediaId":   out.MediaID,
	})
}
