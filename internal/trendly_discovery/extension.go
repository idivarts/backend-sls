package trendlydiscovery

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlybq"
)

// --- S3 + SQS helpers ---

// envS3Origin returns the public https origin for serving S3 objects, e.g. https://cdn.trendly.pro
func envS3Origin() string {
	origin := os.Getenv("S3_ORIGIN")
	if origin == "" {
		// fallback to virtual-hosted–style S3 endpoint if not provided
		origin = "https://s3.amazonaws.com"
	}
	return strings.TrimRight(origin, "/")
}

// envImagesQueueURL returns the SQS queue URL that workers listen on.
func envImagesQueueURL() string {
	return os.Getenv("SQS_IMAGE_QUEUE_URL")
}

// isValidHTTPURL checks if a string is a valid http(s) URL.
func isValidHTTPURL(s string) bool {
	if s == "" {
		return false
	}
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	if u.Host == "" {
		return false
	}
	return true
}

// fileExtFromURL tries to preserve the original file extension if present.
func fileExtFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	ext := path.Ext(u.Path)
	// basic sanity: only allow simple alnum extensions up to 5 chars
	if len(ext) > 0 && len(ext) <= 6 {
		return ext
	}
	return ""
}

// buildS3Path creates a deterministic path using SHA-256 over (kind|username|srcURL).
// Example: instagram/rahul/profile_pic/2f/2f1a....jpg
func buildS3Path(kind, username, srcURL string) string {
	h := sha256.Sum256([]byte(kind + "|" + strings.ToLower(username) + "|" + srcURL))
	hexHash := hex.EncodeToString(h[:])
	ext := fileExtFromURL(srcURL)
	if ext == "" {
		ext = ".jpg"
	}
	// shard into 2-char folders for better distribution
	prefix := hexHash[:2]
	return fmt.Sprintf("instagram/%s/%s/%s/%s%s", strings.ToLower(username), kind, prefix, hexHash, ext)
}

// enqueueImageCopy sends a message to SQS with payload {s3Path, imageUrl}.
// NOTE: Wire this up to AWS SDK v2 where the queue URL is in SQS_IMAGE_QUEUE_URL.
// This function is intentionally left framework-agnostic to avoid import bloat here.
// Replace the TODO block with actual SQS send code in your environment.
func enqueueImageCopy(c *gin.Context, s3Path, imageURL string) error {
	queueURL := envImagesQueueURL()
	if queueURL == "" {
		// If queue is not configured, treat as no-op to avoid failing the write path.
		return nil
	}
	// TODO: Implement using AWS SDK v2:
	//   payload := map[string]string{"s3Path": s3Path, "imageUrl": imageURL}
	//   jsonBody, _ := json.Marshal(payload)
	//   cfg, _ := config.LoadDefaultConfig(c, ...)
	//   client := sqs.NewFromConfig(cfg)
	//   _, err := client.SendMessage(c, &sqs.SendMessageInput{QueueUrl: &queueURL, MessageBody: aws.String(string(jsonBody))})
	//   return err
	return nil
}

// processImageURL validates a source URL, builds the s3 path, enqueues copy, and returns the public CDN URL.
// If srcURL is invalid, it returns srcURL unchanged.
func processImageURL(c *gin.Context, kind, username, srcURL string) string {
	if !isValidHTTPURL(srcURL) {
		return srcURL
	}
	s3Path := buildS3Path(kind, username, srcURL)
	if err := enqueueImageCopy(c, s3Path, srcURL); err != nil {
		// if enqueue fails, fall back to original URL
		return srcURL
	}
	return envS3Origin() + "/" + s3Path
}

func medianInt64(xs []int64) float32 {
	if len(xs) == 0 {
		return 0
	}
	sort.Slice(xs, func(i, j int) bool { return xs[i] < xs[j] })
	n := len(xs)
	if n%2 == 1 {
		return float32(xs[n/2])
	}
	a := xs[n/2-1]
	b := xs[n/2]
	return float32(a+b) / 2
}

func medianFloat32(xs []float32) float32 {
	if len(xs) == 0 {
		return 0
	}
	sort.Slice(xs, func(i, j int) bool { return xs[i] < xs[j] })
	n := len(xs)
	if n%2 == 1 {
		return xs[n/2]
	}
	a := xs[n/2-1]
	b := xs[n/2]
	return (a + b) / 2
}

// ScrapedProfile represents the payload coming from your scraper.
type ScrapedProfile struct {
	SectionsCount int    `json:"sectionsCount" binding:"gte=0"`
	HeaderIndexed bool   `json:"headerIndexed"`
	About         About  `json:"about" binding:"required"`
	Stats         Stats  `json:"stats"`
	Reels         Reels  `json:"reels"`
	Manual        Manual `json:"manual"`
}

// About holds profile "about" info.
type About struct {
	Username    string       `json:"username" binding:"required"`
	FullName    string       `json:"fullName"`
	ProfilePic  string       `json:"profilePic" binding:"omitempty,url"`
	Category    string       `json:"category"`
	Bio         string       `json:"bio"`
	Links       []AboutLink  `json:"links" binding:"dive"`
	MutualsText string       `json:"mutualsText"`
	IsVerified  bool         `json:"isVerified"`
	Actions     AboutActions `json:"actions"`
}

// AboutLink is one entry in the "links" array.
type AboutLink struct {
	Text string `json:"text"`
	URL  string `json:"url" binding:"omitempty,url"`
}

// AboutActions indicates visible CTA buttons.
type AboutActions struct {
	HasFollowButton  bool `json:"hasFollowButton"`
	HasMessageButton bool `json:"hasMessageButton"`
}

// Stats holds high-level counters.
type Stats struct {
	Posts     Metric `json:"posts"`
	Followers Metric `json:"followers"`
	Following Metric `json:"following"`
}

// Metric is a text + numeric value where value may be null.
type Metric struct {
	Text  string `json:"text"`
	Value *int64 `json:"value" binding:"omitempty,gte=0"`
}

// Reels section and items.
type Reels struct {
	Count int        `json:"count" binding:"gte=0"`
	Items []ReelItem `json:"items" binding:"dive"`
}

// ReelItem is one reel card.
type ReelItem struct {
	Index         int          `json:"index" binding:"gte=0"`
	URL           string       `json:"url" binding:"omitempty,url"`
	Thumbnail     string       `json:"thumbnail" binding:"omitempty,url"`
	CoverSizeHint string       `json:"cover_size_hint"`
	Overlays      ReelOverlays `json:"overlays"`
	Views         Metric       `json:"views"` // views.text + views.value (nullable)
	Pinned        bool         `json:"pinned"`
}

// ReelOverlays includes hover overlay + like/comment counts.
type ReelOverlays struct {
	HasHoverOverlay bool   `json:"has_hover_overlay"`
	Likes           Metric `json:"likes"`
	Comments        Metric `json:"comments"`
}

type Manual struct {
	Gender          string   `json:"gender"`
	Niches          []string `json:"niches"`
	Location        string   `json:"location"`
	AestheticsScore int      `json:"aestheticsScore" binding:"gte=0,lte=100"`
}

func AddProfile(c *gin.Context) {
	var req ScrapedProfile
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid Input", "error": err.Error()})
		return
	}

	username := req.About.Username

	adderUserId, b := middlewares.GetUserId(c)
	if !b {
		c.JSON(http.StatusBadRequest, gin.H{"message": "User not authenticated", "error": "UserId not found"})
		return
	}

	checkData := trendlybq.Socials{}
	err := checkData.GetInstagram(req.About.Username)
	if err == nil {
		c.JSON(http.StatusConflict, gin.H{"message": "Profile already exists", "id": checkData.ID})
		return
	}

	data := trendlybq.Socials{
		SocialType:        "instagram",
		Gender:            req.Manual.Gender,
		Niches:            req.Manual.Niches,
		Location:          req.Manual.Location,
		FollowerCount:     *req.Stats.Followers.Value,
		ContentCount:      *req.Stats.Posts.Value,
		FollowingCount:    *req.Stats.Following.Value,
		Username:          req.About.Username,
		Name:              req.About.FullName,
		Bio:               req.About.Bio,
		Category:          req.About.Category,
		ProfilePic:        processImageURL(c, "profile_pic", username, req.About.ProfilePic),
		ProfileVerified:   req.About.IsVerified,
		HasContacts:       len(req.About.Links) > 0,
		HasFollowButton:   req.About.Actions.HasFollowButton,
		HasMessageButton:  req.About.Actions.HasMessageButton,
		ReelScrappedCount: len(req.Reels.Items),
        QualityScore:      req.Manual.AestheticsScore,
        CreationTime:      time.Now().UnixMilli(),
        LastUpdateTime:    time.Now().UnixMicro(),
        AddedBy:           adderUserId,

		ViewsCount:      0,
		EnagamentsCount: 0,

		AverageViews:    0,
		AverageLikes:    0,
		AverageComments: 0,

		EngagementRate: 0,

		Reels: []trendlybq.Reel{},
		Links: []trendlybq.Link{},
	}

	for _, link := range req.About.Links {
		data.Links = append(data.Links, trendlybq.Link{
			URL:  link.URL,
			Text: link.Text,
		})
	}

	eRates := []float32{}
	viewsList := []int64{}
	likesList := []int64{}
	commentsList := []int64{}

	totalLikes := int64(0)
	totalViews := int64(0)
	totalComments := int64(0)

	for index, reel := range req.Reels.Items {
		// Move reel thumbnail to S3 via async SQS
		newThumb := processImageURL(c, "reel_thumb", username, reel.Thumbnail)

		parts := strings.Split(reel.URL, "/")
		id := "reelindex" + strconv.Itoa(index)
		if len(parts) >= 2 {
			id = parts[len(parts)-2]
		}
		data.Reels = append(data.Reels, trendlybq.Reel{
			ID:            id,
			ThumbnailURL:  newThumb,
			URL:           reel.URL,
			Caption:       "",
			Pinned:        reel.Pinned,
			ViewsCount:    bigquery.NullInt64{Int64: 0, Valid: reel.Views.Value != nil && *reel.Views.Value > 0},
			LikesCount:    bigquery.NullInt64{Int64: 0, Valid: reel.Overlays.Likes.Value != nil && *reel.Overlays.Likes.Value > 0},
			CommentsCount: bigquery.NullInt64{Int64: 0, Valid: reel.Overlays.Comments.Value != nil && *reel.Overlays.Comments.Value > 0},
		})

		var views, likes, comments int64

		if reel.Views.Value != nil {
			views = *reel.Views.Value
			if views > 0 {
				data.Reels[len(data.Reels)-1].ViewsCount.Int64 = views
				viewsList = append(viewsList, views)
			}
			if !reel.Pinned {
				data.ViewsCount += views
			}
		}
		if reel.Overlays.Likes.Value != nil {
			likes = *reel.Overlays.Likes.Value
			if likes > 0 {
				data.Reels[len(data.Reels)-1].LikesCount.Int64 = likes
				likesList = append(likesList, likes)
			}
			if !reel.Pinned {
				data.EnagamentsCount += likes
			}
		}
		if reel.Overlays.Comments.Value != nil {
			comments = *reel.Overlays.Comments.Value
			if comments > 0 {
				data.Reels[len(data.Reels)-1].CommentsCount.Int64 = comments
				commentsList = append(commentsList, comments)
			}
			if !reel.Pinned {
				data.EnagamentsCount += comments
			}
		}

		// Per-reel engagement rate for median calculation (treat missing likes/comments as 0)
		if views > 0 {
			eRates = append(eRates, float32(likes+comments)*100/float32(views))
		}

		totalLikes += likes
		totalComments += comments
		totalViews += views
	}

	// Use median for the three "averages"
	data.AverageViews = medianInt64(viewsList)
	data.AverageLikes = medianInt64(likesList)
	data.AverageComments = medianInt64(commentsList)

	// Engagement rate as median of per-reel rates
	data.EngagementRate = medianFloat32(eRates)

	err = data.Insert()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Data Insert Error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"message": "Profile received", "id": data.ID})
}

func CheckUsername(c *gin.Context) {
	username := c.Query("username")
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Username is required"})
		return
	}

	user := trendlybq.Socials{}
	err := user.GetInstagram(username)

	c.JSON(http.StatusAccepted, gin.H{"username": username, "exists": err == nil, "lastUpdate": user.LastUpdateTime})
}
