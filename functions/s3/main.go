package main

import (
	"github.com/TrendsHub/th-backend/internal/videos"
	"github.com/aws/aws-lambda-go/lambda"
)

func main() {
	lambda.Start(videos.S3UploadHandler)
}
