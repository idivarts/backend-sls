package videos

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gin-gonic/gin"
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

func S3UploadHandler(ctx *gin.Context) {
	filename := ctx.Query("filename")
	if filename == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"message": "Filename is required"})
		return
	}
	if !isVideoFile(filename) {
		ctx.JSON(http.StatusBadRequest, gin.H{"message": "Invalid file extension"})
		return
	}

	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
	svc := s3.New(sess)

	// Get s3 bucket name from environment variable
	bucketName := os.Getenv("VIDEO_S3_BUCKET_NAME")
	domainUrl := os.Getenv("CLOUDFRONT_DISTRIBUTION_URL")

	filename = fmt.Sprintf("file_%d_%s", time.Now().Unix(), filename)

	bucketKey := fmt.Sprintf("raw_videos/%s", filename)
	videoUrl := fmt.Sprintf("%s/%s", domainUrl, bucketKey)

	// // Get the file extension
	// ext := filepath.Ext(filename)
	// // Remove the extension from the filename
	// fileWithoutExtension := strings.TrimSuffix(filename, ext)

	// appleUrl := fmt.Sprintf("%s/outputs/%s.m3u8", domainUrl, fileWithoutExtension)
	// playUrl := fmt.Sprintf("%s/outputs/%s.mpd", domainUrl, fileWithoutExtension)

	req, _ := svc.PutObjectRequest(&s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    &bucketKey,
	})
	url, err := req.Presign(15 * time.Minute)
	if err != nil {
		log.Println("Failed to sign request", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"uploadUrl": url, "appleUrl": videoUrl, "playUrl": videoUrl})
}
