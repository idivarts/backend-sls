package trendlydiscovery_test

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDiscovery(t *testing.T) {
	// sql := trendlydiscovery.FormSQL(trendlydiscovery.InfluencerFilters{
	// 	FollowerMin: aws.Int64(7000),
	// 	Name:        aws.String("Saks"),
	// })
	// log.Println(sql)
}

func TestDownloadImage(t *testing.T) {
	url := "https://scontent-ccu1-1.cdninstagram.com/v/t51.2885-19/527096233_18308167444169520_3036518684579133625_n.jpg?efg=eyJ2ZW5jb2RlX3RhZyI6InByb2ZpbGVfcGljLmRqYW5nby4xMDgwLmMyIn0&_nc_ht=scontent-ccu1-1.cdninstagram.com&_nc_cat=100&_nc_oc=Q6cZ2QGO59Mvl0lRRh9a1FeVQlkQlGRj3A8jWvHwb2I7u9KQYSwbkHGOrxpsmPpD1rj8xAQ&_nc_ohc=bu45eU1cy7IQ7kNvwEsFCIF&_nc_gid=VlOWfnx5H1-1jr-RoLonyw&edm=ALGbJPMBAAAA&ccb=7-5&oh=00_AfbI7ZXvEP7g_Nru3AUFrvI_i261bgY7RUTmU4bfPvqdlA&oe=68C51588&_nc_sid=7d3ac5"

	location, err := DownloadImage((url))
	if err != nil {
		panic(err)
	}

	t.Log("Location", location)
}

// contentTypeToExt returns a common file extension for a limited set of image MIME types.
func contentTypeToExt(ct string) string {
	ct = strings.ToLower(strings.TrimSpace(ct))
	switch {
	case strings.HasPrefix(ct, "image/jpeg"), strings.HasPrefix(ct, "image/jpg"):
		return ".jpg"
	case strings.HasPrefix(ct, "image/png"):
		return ".png"
	case strings.HasPrefix(ct, "image/webp"):
		return ".webp"
	case strings.HasPrefix(ct, "image/gif"):
		return ".gif"
	default:
		return ""
	}
}

// DownloadImage downloads an image from the given URL and saves it locally in the OS temp directory.
// It returns the absolute path to the saved file, or an error.
func DownloadImage(rawURL string) (string, error) {
	return DownloadImageTo(rawURL, os.TempDir())
}

// DownloadImageTo downloads an image from rawURL and saves it inside dir. The file name is derived
// from a SHA-1 hash of the URL and current time, plus an extension inferred from the URL path or Content-Type.
func DownloadImageTo(rawURL, dir string) (string, error) {
	if rawURL == "" {
		return "", fmt.Errorf("empty url")
	}

	u, err := neturl.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("invalid url: %w", err)
	}

	// Build request
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	// Use a common browser UA to avoid upstream blocks
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; TrendlyImageFetcher/1.0)")

	// Fetch
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("upstream responded with %d", resp.StatusCode)
	}

	// Decide on extension
	ext := filepath.Ext(u.Path)
	if ext == "" {
		ext = contentTypeToExt(resp.Header.Get("Content-Type"))
		if ext == "" {
			// safe fallback
			ext = ".img"
		}
	}

	// Create deterministic-ish file name
	h := sha1.New()
	io.WriteString(h, rawURL)
	io.WriteString(h, time.Now().UTC().Format(time.RFC3339Nano))
	name := hex.EncodeToString(h.Sum(nil)) + ext

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dstPath := filepath.Join(dir, name)

	// Create and copy
	f, err := os.Create(dstPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", err
	}

	return dstPath, nil
}
