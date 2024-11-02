package main

import (
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/s3/attachments"
	"github.com/idivarts/backend-sls/internal/s3/images"
	"github.com/idivarts/backend-sls/internal/s3/videos"
	apihandler "github.com/idivarts/backend-sls/pkg/api_handler"
)

func main() {
	apiV1 := apihandler.GinEngine.Group("/s3/v1", middlewares.ValidateSessionMiddleware())

	apiV1.POST("/videos", videos.S3UploadHandler)
	apiV1.POST("/images", images.S3UploadHandler)
	apiV1.POST("/attachments", attachments.S3UploadHandler)

	apihandler.StartLambda()
}
