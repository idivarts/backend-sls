package main

import (
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/idivarts/backend-sls/internal/s3/videos"
)

func main() {
	lambda.Start(videos.VideoProcessHandler)
}
