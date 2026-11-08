// Package attachments hands out pre-signed uploads for contract and chat files.
//
// It routes through pkg/mediastore so the codebase has exactly one S3 writer,
// but it files NO library row, and that is deliberate: a document a brand sends
// an influencer is not the brand's media. Filing it would offer someone's
// signed agreement in the scene picker.
package attachments

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/pkg/mediastore"
)

func S3UploadHandler(ctx *gin.Context) {
	filename := ctx.Query("filename")
	if filename == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"message": "Filename is required"})
		return
	}

	out, err := mediastore.PresignUpload(ctx.Request.Context(), mediastore.Spec{
		Target:   mediastore.TargetAttachment,
		Filename: filename,
		// Not the brand's media: a contract or a chat file, which must never
		// turn up in the scene picker.
		NoLibrary: true,
	})
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"uploadUrl": out.UploadURL, "attachmentUrl": out.URL})
}
