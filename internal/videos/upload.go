package videos

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
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

func S3UploadHandler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	filename := request.QueryStringParameters["filename"]
	if filename == "" {
		return events.APIGatewayProxyResponse{StatusCode: http.StatusBadRequest, Body: "Filename is required"}, nil
	}
	if !isVideoFile(filename) {
		return events.APIGatewayProxyResponse{StatusCode: http.StatusBadRequest, Body: "File needs to be a video"}, nil
	}

	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
	svc := s3.New(sess)

	// Get s3 bucket name from environment variable
	bucketName := os.Getenv("VIDEO_S3_BUCKET_NAME")
	domainUrl := os.Getenv("CLOUDFRONT_DISTRIBUTION_URL")

	filename = fmt.Sprintf("file_%d_%s", time.Now().Unix(), filename)
	// Get the file extension
	ext := filepath.Ext(filename)
	// Remove the extension from the filename
	fileWithoutExtension := strings.TrimSuffix(filename, ext)

	appleUrl := fmt.Sprintf("%s/outputs/%s.m3u8", domainUrl, fileWithoutExtension)
	playUrl := fmt.Sprintf("%s/outputs/%s.mpd", domainUrl, fileWithoutExtension)

	req, _ := svc.PutObjectRequest(&s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(fmt.Sprintf("uploads/%s", filename)),
	})
	url, err := req.Presign(15 * time.Minute)
	if err != nil {
		log.Println("Failed to sign request", err)
		return events.APIGatewayProxyResponse{StatusCode: http.StatusInternalServerError}, nil
	}

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Body:       fmt.Sprintf(`{"uploadUrl": "%s", "appleUrl": "%s", "playUrl":"%s"}`, url, appleUrl, playUrl),
	}, nil
}
