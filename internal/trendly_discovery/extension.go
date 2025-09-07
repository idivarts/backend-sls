package trendlydiscovery

import (
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlybq"
)

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

	adderUserId, b := middlewares.GetUserId(c)
	if !b {
		c.JSON(http.StatusBadRequest, gin.H{"message": "User not authenticated", "error": "UserId not found"})
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
		ProfilePic:        req.About.ProfilePic,
		ProfileVerified:   req.About.IsVerified,
		HasContacts:       len(req.About.Links) > 0,
		HasFollowButton:   req.About.Actions.HasFollowButton,
		HasMessageButton:  req.About.Actions.HasMessageButton,
		ReelScrappedCount: len(req.Reels.Items),
		QualityScore:      req.Manual.AestheticsScore,
		CreationTime:      time.Now().UnixMicro(), // TODO: set actual creation time
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
	totalLikes := int64(0)
	totalViews := int64(0)
	totalComments := int64(0)
	for _, reel := range req.Reels.Items {
		parts := strings.Split(reel.URL, "/")
		data.Reels = append(data.Reels, trendlybq.Reel{
			ID:            parts[len(parts)-1],
			ThumbnailURL:  reel.Thumbnail,
			URL:           reel.URL,
			Caption:       "",
			Pinned:        reel.Pinned,
			ViewsCount:    bigquery.NullInt64{Int64: 0, Valid: reel.Views.Value != nil},
			LikesCount:    bigquery.NullInt64{Int64: 0, Valid: reel.Overlays.Likes.Value != nil},
			CommentsCount: bigquery.NullInt64{Int64: 0, Valid: reel.Overlays.Comments.Value != nil},
		})
		var views, likes, comments int64

		if reel.Views.Value != nil {
			if !reel.Pinned {
				data.ViewsCount += *reel.Views.Value
			}
			views = *reel.Views.Value
			data.Reels[len(data.Reels)-1].ViewsCount.Int64 = views
		}
		if reel.Overlays.Likes.Value != nil {
			if !reel.Pinned {
				data.EnagamentsCount += *reel.Overlays.Likes.Value
			}
			likes = *reel.Overlays.Likes.Value
			data.Reels[len(data.Reels)-1].LikesCount.Int64 = likes
		}
		if reel.Overlays.Comments.Value != nil {
			if !reel.Pinned {
				data.EnagamentsCount += *reel.Overlays.Comments.Value
			}
			comments = *reel.Overlays.Comments.Value
			data.Reels[len(data.Reels)-1].CommentsCount.Int64 = comments
		}
		if views != 0 {
			eRates = append(eRates, float32(likes+comments)*100/float32(views))
		} else {
			eRates = append(eRates, 0)
		}
		totalLikes += likes
		totalComments += comments
		totalViews += views
	}

	data.AverageViews = float32(totalViews) / float32(len(req.Reels.Items))
	data.AverageLikes = float32(totalLikes) / float32(len(req.Reels.Items))
	data.AverageComments = float32(totalComments) / float32(len(req.Reels.Items))

	c.JSON(http.StatusAccepted, gin.H{"message": "Profile received", "data": data})
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
