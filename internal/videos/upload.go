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

func S3UploadHandler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	filename := request.QueryStringParameters["filename"]
	if filename == "" {
		return events.APIGatewayProxyResponse{StatusCode: http.StatusBadRequest, Body: "Filename is required"}, nil
	}

	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
	svc := s3.New(sess)

	// Get s3 bucket name from environment variable
	bucketName := os.Getenv("VIDEO_S3_BUCKET_NAME")
	domainUrl := os.Getenv("CLOUDFRONT_DISTRIBUTION_URL")

	// Get the file extension
	ext := filepath.Ext(filename)
	// Remove the extension from the filename
	fileWithoutExtension := strings.TrimSuffix(filename, ext)

	fetchUrl := fmt.Sprintf("%s/outputs/%s.m3u8", domainUrl, fileWithoutExtension)

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
		Body:       fmt.Sprintf(`{"uploadUrl": "%s", "fetchUrl": "%s"}`, url, fetchUrl),
	}, nil
}
