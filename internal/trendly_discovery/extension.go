package trendlydiscovery

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ScrapedProfile represents the payload coming from your scraper.
type ScrapedProfile struct {
	SectionsCount int   `json:"sectionsCount" binding:"gte=0"`
	HeaderIndexed bool  `json:"headerIndexed"`
	About         About `json:"about" binding:"required"`
	Stats         Stats `json:"stats"`
	Reels         Reels `json:"reels"`
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

func AddProfile(c *gin.Context) {
	var req ScrapedProfile
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid Input", "error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"message": "Profile received", "data": req})
}

func CheckUsername(c *gin.Context) {
	username := c.Query("username")
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Username is required"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"username": username, "exists": false, "lastUpdate": nil})
}
