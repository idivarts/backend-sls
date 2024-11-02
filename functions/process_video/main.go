package main

import (
	"github.com/TrendsHub/th-backend/internal/s3/videos"
	"github.com/aws/aws-lambda-go/lambda"
)

func main() {
	lambda.Start(videos.VideoProcessHandler)
}
