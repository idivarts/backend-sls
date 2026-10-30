package mediajobs

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/google/uuid"
	"github.com/idivarts/backend-sls/pkg/mys3"
)

// hostFromURL copies a provider's output into our own bucket and returns the
// CDN url.
//
// Not optional for generated video. The provider hands back a SIGNED url that
// expires about an hour out, and a composition is a document people re-render
// weeks later — a clip pointing at an expired url is a timeline nobody can
// render again.
//
// Follows the same convention as the apps' own pre-signed uploads: same bucket,
// the shared `uploads/` prefix, the same CloudFront host. A generated clip then
// lives alongside the user's own files and needs no special handling anywhere
// downstream.
func hostFromURL(url, brandID string) (string, error) {
	resp, err := downloadClient.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("mediajobs: download returned %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	contentType := resp.Header.Get("Content-Type")
	ext := "mp4"
	switch {
	case strings.Contains(contentType, "webm"):
		ext = "webm"
	case strings.Contains(contentType, "quicktime"):
		ext = "mov"
	}
	if contentType == "" {
		contentType = "video/mp4"
	}

	bucket := os.Getenv("IMAGE_S3_BUCKET_NAME")
	cdn := strings.TrimRight(os.Getenv("IMAGE_CF_DISTRIBUTION_URL"), "/")
	if bucket == "" || cdn == "" {
		return "", fmt.Errorf("mediajobs: IMAGE_S3_BUCKET_NAME / IMAGE_CF_DISTRIBUTION_URL are not set")
	}
	key := fmt.Sprintf("uploads/file_%d_clip_%s.%s", time.Now().Unix(), uuid.NewString(), ext)
	if _, err := mys3.Client.PutObject(&s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	}); err != nil {
		return "", err
	}
	return cdn + "/" + key, nil
}

// downloadClient is generous on time: a finished clip is tens of megabytes and
// comes from a provider's CDN, not ours.
var downloadClient = &http.Client{Timeout: 5 * time.Minute}
