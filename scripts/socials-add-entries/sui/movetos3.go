package sui

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
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/idivarts/backend-sls/internal/models/trendlyrdb"
)

// s3Task represents a single download-and-upload job.
type s3Task struct {
	url       string
	prefix    string
	setResult func(string)
}

// maxS3Concurrency caps parallel S3 uploads to avoid exhausting file
// descriptors, memory, or overwhelming the upstream CDN / S3 rate limits.
// 10 is a safe default for Lambda-sized environments.
const maxS3Concurrency = 10

// MoveImagesToS3 downloads all image URLs found in the social profile and its
// posts, re-uploads them to S3, and rewrites the URLs in-place.
// Downloads run concurrently (up to maxS3Concurrency at a time).
func MoveImagesToS3(social *trendlyrdb.Socials, posts []trendlyrdb.InstagramPost) (*trendlyrdb.Socials, []trendlyrdb.InstagramPost) {
	var tasks []s3Task

	// --- Social profile images ---
	if social.ProfilePic != "" {
		tasks = append(tasks, s3Task{
			url:       social.ProfilePic,
			prefix:    fmt.Sprintf("instagram/%s/profile-", social.ID),
			setResult: func(url string) { social.ProfilePic = url },
		})
	}
	if social.ProfilePicHD != "" {
		tasks = append(tasks, s3Task{
			url:       social.ProfilePicHD,
			prefix:    fmt.Sprintf("instagram/%s/profile-hd-", social.ID),
			setResult: func(url string) { social.ProfilePicHD = url },
		})
	}

	// --- Post images ---
	for i := range posts {
		idx := i // capture for closure
		if posts[idx].DisplayURL != "" {
			tasks = append(tasks, s3Task{
				url:       posts[idx].DisplayURL,
				prefix:    fmt.Sprintf("instagram/%s/posts-", social.ID),
				setResult: func(url string) { posts[idx].DisplayURL = url },
			})
		}
		for j := range posts[idx].Images {
			jj := j // capture for closure
			if posts[idx].Images[jj] != "" {
				tasks = append(tasks, s3Task{
					url:       posts[idx].Images[jj],
					prefix:    fmt.Sprintf("instagram/%s/posts-", social.ID),
					setResult: func(url string) { posts[idx].Images[jj] = url },
				})
			}
		}
	}

	// --- Execute concurrently with a semaphore ---
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxS3Concurrency)

	for _, task := range tasks {
		wg.Add(1)
		go func(t s3Task) {
			defer wg.Done()
			sem <- struct{}{}        // acquire slot
			defer func() { <-sem }() // release slot

			p, err := DownloadAndUploadToS3(t.url, t.prefix)
			if err != nil {
				log.Println("Error uploading to S3:", t.url, err.Error())
				return
			}
			t.setResult(p)
		}(task)
	}

	wg.Wait()
	log.Println("Successfully uploaded all images to S3")
	return social, posts
}

// DownloadAndUploadToS3 downloads the image from URL, saves it to /tmp, and uploads to S3.
// Returns the public S3 URL.
func DownloadAndUploadToS3(url, path string) (string, error) {
	S3Bucket := os.Getenv("S3_BUCKET") // better than ARN for uploading
	S3URL := os.Getenv("S3_URL")

	if strings.HasPrefix(url, S3URL) {
		return url, nil
	}

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
