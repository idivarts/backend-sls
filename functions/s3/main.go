package main

import (
	"github.com/TrendsHub/th-backend/internal/middlewares"
	"github.com/TrendsHub/th-backend/internal/s3/videos"
	apihandler "github.com/TrendsHub/th-backend/pkg/api_handler"
)

func main() {
	apiV1 := apihandler.GinEngine.Group("/s3/v1", middlewares.ValidateSessionMiddleware())

	apiV1.POST("/videos", videos.S3UploadHandler)
	apiV1.POST("/images", videos.S3UploadHandler)
	apiV1.POST("/attachments", videos.S3UploadHandler)

	apihandler.StartLambda()
}
