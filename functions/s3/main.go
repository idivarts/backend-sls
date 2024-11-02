package main

import (
	"github.com/TrendsHub/th-backend/internal/middlewares"
	"github.com/TrendsHub/th-backend/internal/s3/attachments"
	"github.com/TrendsHub/th-backend/internal/s3/images"
	"github.com/TrendsHub/th-backend/internal/s3/videos"
	apihandler "github.com/TrendsHub/th-backend/pkg/api_handler"
)

func main() {
	apiV1 := apihandler.GinEngine.Group("/s3/v1", middlewares.ValidateSessionMiddleware())

	apiV1.POST("/videos", videos.S3UploadHandler)
	apiV1.POST("/images", images.S3UploadHandler)
	apiV1.POST("/attachments", attachments.S3UploadHandler)

	apihandler.StartLambda()
}
