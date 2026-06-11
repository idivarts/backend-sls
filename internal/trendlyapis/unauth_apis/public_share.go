package trendlyunauth

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

type publicCalendarItem struct {
	ID               string `json:"id"`
	Title            string `json:"title"`
	ContentFormat    string `json:"contentFormat,omitempty"`
	Platform         string `json:"platform,omitempty"`
	Status           string `json:"status,omitempty"`
	PostingTimeStamp int64  `json:"postingTimeStamp,omitempty"`
	ImageURL         string `json:"imageUrl,omitempty"`
}

type publicBrandInfo struct {
	Name  string `json:"name,omitempty"`
	Image string `json:"image,omitempty"`
}

// PublicShareResolve serves an unauthenticated, view-only payload for a public
// share link. Strategy and single-content shares are read client-side directly
// from Firestore (gated by rules); this endpoint exists for the Calendar month
// share, whose data is a query across many content documents that the client
// rules can't cleanly gate.
//
//	GET /public/shares/:token
func PublicShareResolve(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing token"})
		return
	}

	ctx := context.Background()

	// 1. Resolve + validate the share link.
	link, err := trendlymodels.GetShareLink(ctx, token)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "link not found"})
		return
	}
	if !link.Enabled {
		c.JSON(http.StatusNotFound, gin.H{"error": "link disabled"})
		return
	}

	// 2. Only calendar-month shares are served here.
	if link.Type != "calendarMonth" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported share type"})
		return
	}
	if link.BrandID == "" || link.Month == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "invalid share"})
		return
	}

	// 3. Derive the month window [start, end) in epoch ms.
	monthStart, err := time.Parse("2006-01", link.Month)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid month"})
		return
	}
	start := monthStart.UnixMilli()
	end := monthStart.AddDate(0, 1, 0).UnixMilli()

	// 4. Query the month's (non-archived) content items.
	contents, err := trendlymodels.ListContentInRange(ctx, link.BrandID, start, end, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load calendar"})
		return
	}

	items := []publicCalendarItem{}
	for _, ct := range contents {
		imageURL := ""
		for _, a := range ct.Attachments {
			if a.ImageURL != "" {
				imageURL = a.ImageURL
				break
			}
		}
		items = append(items, publicCalendarItem{
			ID:               ct.ID,
			Title:            ct.Title,
			ContentFormat:    ct.ContentFormat,
			Platform:         ct.Platform,
			Status:           ct.Status,
			PostingTimeStamp: ct.PostingTimeStamp,
			ImageURL:         imageURL,
		})
	}

	// 5. Attach lightweight brand display info (best-effort).
	brandInfo := publicBrandInfo{}
	brand := trendlymodels.Brand{}
	if err := brand.Get(link.BrandID); err == nil {
		brandInfo.Name = brand.Name
		if brand.Image != nil {
			brandInfo.Image = *brand.Image
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"brand": brandInfo,
		"month": link.Month,
		"items": items,
	})
}
