package trendly_discovery_sqs

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/idivarts/backend-sls/internal/models/trendlybq"
)

func MoveImagesToS3(socialId string) {
	social := &trendlybq.Socials{}

	err := social.Get(socialId)
	if err != nil {
		log.Println("Error Getting Social", socialId, err.Error())
		return
	}

}

// DownloadAndUploadToS3 downloads the image from URL, saves it to /tmp, and uploads to S3.
// Returns the public S3 URL.
func DownloadAndUploadToS3(url, path string) (string, error) {
	S3Bucket := os.Getenv("S3_BUCKET") // better than ARN for uploading
	S3URL := os.Getenv("S3_URL")

	// ---- Step 1: Download the image ----
	resp, err := http.Get(url)
	if err != nil {
		log.Println("Error downloading image:", err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Println("Failed to download image, status:", resp.Status)
		return "", errors.New("status-error")
	}

	// ---- Step 2: Save to temporary file ----
	tmpFile, err := os.CreateTemp("", "img-*.tmp")
	if err != nil {
		log.Println("Error creating temp file:", err)
		return "", err
	}
	defer os.Remove(tmpFile.Name()) // clean up
	defer tmpFile.Close()

	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		log.Println("Error writing to temp file:", err)
		return "", err
	}

	// Reset file pointer for upload
	_, err = tmpFile.Seek(0, io.SeekStart)
	if err != nil {
		log.Println("Error seeking temp file:", err)
		return "", err
	}

	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))

	// Create an SQS service client
	client := s3.New(sess)

	// Generate object key
	filename := filepath.Base(tmpFile.Name()) + filepath.Ext(url)
	key := path + filename

	_, err = client.PutObject(&s3.PutObjectInput{
		Bucket: aws.String(S3Bucket),
		Key:    aws.String(key),
		Body:   tmpFile,
	})
	if err != nil {
		log.Println("Error uploading to S3:", err)
		return "", err
	}

	// ---- Step 4: Return S3 URL ----
	return fmt.Sprintf("%s/%s", S3URL, key), nil
}

// S3_BUCKET: trendly-discovery-bucket
// S3_URL: https://trendly-discovery-bucket.s3.us-east-1.amazonaws.com
