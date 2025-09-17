package trendly_discovery_sqs

import (
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	neturl "net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

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

	if social.ProfilePic != "" {
		p, err := DownloadAndUploadToS3(social.ProfilePic, fmt.Sprintf("instagram/%s/profile-", social.ID))
		if err != nil {
			log.Println("Error Uploading Profile Pic", socialId, err.Error())
		} else {
			social.ProfilePic = p
		}
	}

	for i, v := range social.Reels {
		if v.ThumbnailURL != "" {
			p, err := DownloadAndUploadToS3(v.ThumbnailURL, fmt.Sprintf("instagram/%s/reels-", social.ID))
			if err != nil {
				log.Println("Error Uploading Reel Pic", socialId, v.ID, err.Error())
			} else {
				social.Reels[i].ThumbnailURL = p
			}
		}
	}

	err = social.Update()
	if err != nil {
		log.Println("Error Updating Social", socialId, err.Error())
		return
	}
	log.Println("Success")
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
	// Derive a stable filename using the URL's path-based extension (ignoring query params).
	// Fallback to Content-Type from the response headers if the URL has no extension.
	base := strings.TrimSuffix(filepath.Base(tmpFile.Name()), filepath.Ext(tmpFile.Name()))
	ext := fileExtFromURL(url)
	if ext == "" {
		if ctype := resp.Header.Get("Content-Type"); ctype != "" {
			if exts, _ := mime.ExtensionsByType(ctype); len(exts) > 0 {
				ext = exts[0]
			}
		}
	}
	if ext == "" {
		ext = ".bin"
	}
	filename := base + ext

	key := path + filename

	ct := resp.Header.Get("Content-Type")

	_, err = client.PutObject(&s3.PutObjectInput{
		Bucket:      aws.String(S3Bucket),
		Key:         aws.String(key),
		Body:        tmpFile,
		ContentType: aws.String(ct),
	})
	if err != nil {
		log.Println("Error uploading to S3:", err)
		return "", err
	}

	// ---- Step 4: Return S3 URL ----
	return fmt.Sprintf("%s/%s", S3URL, key), nil
}

// fileExtFromURL extracts the extension from a URL path (ignoring query params and fragments).
// It returns a lowercase extension like ".jpg" or an empty string if none is found.
func fileExtFromURL(rawURL string) string {
	u, err := neturl.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(path.Ext(u.Path))
}

// S3_BUCKET: trendly-discovery-bucket
// S3_URL: https://trendly-discovery-bucket.s3.us-east-1.amazonaws.com
